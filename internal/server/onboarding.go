package server

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/onboarding"
)

type onboardingMessageInput struct {
	Message string `json:"message"`
}

func (a *App) getOnboardingInterview(context *gin.Context) {
	owner := sessionAccount(context)
	state, err := a.onboarding.State(context, owner.ID)
	if err != nil {
		writeInternalError(context, "read Onboarding interview", err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) sendOnboardingMessage(context *gin.Context) {
	owner := sessionAccount(context)
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4<<10)
	var input onboardingMessageInput
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "消息格式无效")
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" || utf8.RuneCountInString(input.Message) > onboarding.MaxMessageRunes {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "消息须为 1–600 个字符")
		return
	}
	state, err := a.onboarding.Send(context, owner.ID, input.Message)
	if err != nil {
		a.writeOnboardingError(context, owner.ID, err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) retryOnboardingInterview(context *gin.Context) {
	owner := sessionAccount(context)
	state, err := a.onboarding.Retry(context, owner.ID)
	if err != nil {
		a.writeOnboardingError(context, owner.ID, err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) useManualOnboarding(context *gin.Context) {
	owner := sessionAccount(context)
	state, err := a.onboarding.Manual(context, owner.ID)
	if err != nil {
		a.writeOnboardingError(context, owner.ID, err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) writeOnboardingError(
	context *gin.Context,
	accountID int64,
	err error,
) {
	switch {
	case errors.Is(err, onboarding.ErrNIMUnavailable):
		log.Printf("NIM Onboarding call failed for Account %d: %v", accountID, err)
		writeError(
			context,
			http.StatusServiceUnavailable,
			codeNIMUnavailable,
			"NIM 暂时不可用，请重试或改用手工 Catalog 编辑",
		)
	case errors.Is(err, onboarding.ErrRateLimited):
		context.Header("Retry-After", strconv.Itoa(int(onboarding.RatePeriod.Seconds())))
		writeError(
			context,
			http.StatusTooManyRequests,
			codeRateLimited,
			"访谈消息过于频繁，请稍后再试",
		)
	case errors.Is(err, onboarding.ErrAttemptLimitReached):
		writeError(
			context,
			http.StatusTooManyRequests,
			codeInterviewLimitReached,
			"访谈次数已达上限，请改用手工 Catalog 编辑",
		)
	case errors.Is(err, onboarding.ErrRetryRequired):
		writeError(
			context,
			http.StatusConflict,
			codeRetryRequired,
			"上一条消息尚未完成，请先重试",
		)
	case errors.Is(err, onboarding.ErrRetryUnavailable):
		writeError(
			context,
			http.StatusConflict,
			codeRetryUnavailable,
			"当前没有可重试的访谈消息",
		)
	case errors.Is(err, onboarding.ErrFinished):
		writeError(
			context,
			http.StatusConflict,
			codeInterviewFinished,
			"Onboarding interview 已结束",
		)
	default:
		writeInternalError(context, "update Onboarding interview", err)
	}
}
