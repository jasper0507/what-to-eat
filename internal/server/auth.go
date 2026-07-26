package server

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/account"
)

const (
	sessionCookieName = "what2eat_session"
	sessionAccountKey = "what2eat_account"
)

// requireAccount 是 account.Sessions 在 HTTP seam 上的 adapter：校验会话并
// 把 Account 放进请求上下文，之后的 handler 用 sessionAccount 读取。
func (a *App) requireAccount(context *gin.Context) {
	cookie, err := context.Cookie(sessionCookieName)
	if err != nil {
		writeError(context, http.StatusUnauthorized, codeUnauthorized, "需要登录")
		context.Abort()
		return
	}
	owner, err := a.sessions.Verify(context, cookie)
	if errors.Is(err, account.ErrUnauthenticated) {
		writeError(context, http.StatusUnauthorized, codeUnauthorized, "需要登录")
		context.Abort()
		return
	}
	if err != nil {
		writeInternalError(context, "restore Account session", err)
		context.Abort()
		return
	}
	context.Set(sessionAccountKey, owner)
}

func sessionAccount(context *gin.Context) account.Account {
	owner, _ := context.MustGet(sessionAccountKey).(account.Account)
	return owner
}

func (a *App) register(context *gin.Context) {
	var input account.Credentials
	if err := context.ShouldBindJSON(&input); err != nil || !account.ValidCredentials(input) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "用户名或密码不符合要求")
		return
	}

	grant, err := a.sessions.Register(context, input)
	if errors.Is(err, account.ErrPasswordUnusable) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "用户名或密码不符合要求")
		return
	}
	if errors.Is(err, account.ErrUnavailable) {
		writeError(context, http.StatusConflict, codeAccountUnavailable, "无法创建 Account")
		return
	}
	if err != nil {
		writeInternalError(context, "register Account", err)
		return
	}

	a.setSessionCookie(context, grant.Token, grant.ExpiresAt)
	context.JSON(http.StatusCreated, gin.H{"account": grant.Account})
}

func (a *App) login(context *gin.Context) {
	var input account.Credentials
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "请求格式无效")
		return
	}

	grant, err := a.sessions.Login(context, input, requestIP(context.Request))
	switch {
	case errors.Is(err, account.ErrLoginRateLimited):
		writeError(context, http.StatusTooManyRequests, codeRateLimited, "登录尝试过多，请稍后再试")
	case errors.Is(err, account.ErrInvalidCredentials):
		writeError(context, http.StatusUnauthorized, codeInvalidCredentials, "用户名或密码错误")
	case err != nil:
		writeInternalError(context, "login Account", err)
	default:
		a.setSessionCookie(context, grant.Token, grant.ExpiresAt)
		context.JSON(http.StatusOK, gin.H{"account": grant.Account})
	}
}

func (a *App) session(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{"account": sessionAccount(context)})
}

// logout 幂等注销：没 cookie、token 已失效都返回 204——登出的目的只是
// 「这台设备不再持有会话」，不需要先证明会话有效。
func (a *App) logout(context *gin.Context) {
	if cookie, err := context.Cookie(sessionCookieName); err == nil {
		if err := a.sessions.Revoke(context, cookie); err != nil {
			writeInternalError(context, "revoke Account session", err)
			return
		}
	}
	a.clearSessionCookie(context)
	context.Status(http.StatusNoContent)
}

func (a *App) setSessionCookie(context *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(context.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) clearSessionCookie(context *gin.Context) {
	http.SetCookie(context.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func requestIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}
