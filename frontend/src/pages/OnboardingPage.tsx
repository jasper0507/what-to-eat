import { Button, Input } from "antd";
import { useEffect, useRef, useState } from "react";
import { Navigate } from "react-router-dom";

import {
  useManualOnboarding,
  useOnboardingState,
  useRetryOnboarding,
  useSendOnboardingMessage,
} from "@/api/hooks";
import { ChatBubble } from "@/components/ChatBubble";
import { ErrorAlert } from "@/components/ErrorAlert";
import { LoadingBlock } from "@/components/LoadingBlock";
import { PageHeader } from "@/components/PageHeader";
import { copy } from "@/lib/copy";
import { useRetryAfter } from "@/lib/useRetryAfter";
import { MESSAGE_MAX_RUNES } from "@/lib/validation";

export default function OnboardingPage() {
  const onboarding = useOnboardingState();
  const send = useSendOnboardingMessage();
  const retry = useRetryOnboarding();
  const manual = useManualOnboarding();

  const [draft, setDraft] = useState("");
  const inputRef = useRef<import("antd").InputRef>(null);
  const messagesRef = useRef<HTMLDivElement>(null);

  const messages = onboarding.data?.messages ?? [];
  const canRetry = onboarding.data?.can_retry ?? false;
  const busy = send.isPending || retry.isPending || manual.isPending;
  // 同一时刻只渲染一个错误（规格用正则匹配文案，双份会触发 strict violation）
  const activeError = retry.error ?? manual.error ?? send.error;
  const retryRemaining = useRetryAfter(activeError);

  // 新消息到达后滚到底部
  useEffect(() => {
    const container = messagesRef.current;
    if (!container) {
      return;
    }
    const reduce = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    container.scrollTo({
      top: container.scrollHeight,
      behavior: reduce ? "auto" : "smooth",
    });
  }, [messages.length]);

  // 反向门控 + 完成跳转合一：completed / manual 一律去 Candidate pool。
  // 访谈在 mutation 成功后写入缓存，本组件重渲染即声明式跳转——无竞态。
  if (
    onboarding.data &&
    (onboarding.data.status === "completed" ||
      onboarding.data.status === "manual")
  ) {
    return <Navigate to="/candidate-pool" replace />;
  }

  const submitDraft = () => {
    const message = draft.trim();
    if (message === "" || busy || canRetry) {
      return;
    }
    retry.reset();
    manual.reset();
    send.mutate(message, {
      onSuccess: () => {
        setDraft("");
        inputRef.current?.focus();
      },
    });
  };

  const runRetry = () => {
    send.reset();
    manual.reset();
    retry.mutate();
  };

  const runManual = () => {
    send.reset();
    retry.reset();
    manual.mutate();
  };

  return (
    <div className="container chat-page">
      <PageHeader title={copy.onboarding.title} intro={copy.onboarding.intro} />

      {onboarding.isPending ? (
        <LoadingBlock preset="chat" label={copy.onboarding.loadingLabel} />
      ) : null}
      {onboarding.isError ? (
        <ErrorAlert
          error={onboarding.error}
          onRetry={() => void onboarding.refetch()}
        />
      ) : null}

      {onboarding.data ? (
        <>
          <div className="chat-messages" ref={messagesRef}>
            {messages.length === 0 ? (
              <ChatBubble role="assistant">
                {copy.onboarding.seedMessage}
              </ChatBubble>
            ) : null}
            {messages.map((message, index) => (
              <ChatBubble key={`${index}:${message.role}`} role={message.role}>
                {message.content}
              </ChatBubble>
            ))}
          </div>

          <div className="chat-composer">
            <ErrorAlert error={activeError} retryRemaining={retryRemaining} />
            <Input.TextArea
              ref={inputRef}
              aria-label={copy.onboarding.inputLabel}
              autoSize={{ minRows: 3, maxRows: 6 }}
              maxLength={MESSAGE_MAX_RUNES}
              value={draft}
              disabled={busy || canRetry}
              onChange={(event) => setDraft(event.target.value)}
              onPressEnter={(event) => {
                if (!event.shiftKey) {
                  event.preventDefault();
                  submitDraft();
                }
              }}
            />
            {canRetry ? (
              <Button
                block
                type="primary"
                size="large"
                loading={retry.isPending}
                disabled={(busy && !retry.isPending) || retryRemaining > 0}
                onClick={runRetry}
              >
                {copy.onboarding.retry}
              </Button>
            ) : (
              <Button
                block
                type="primary"
                size="large"
                loading={send.isPending}
                disabled={
                  draft.trim() === "" ||
                  (busy && !send.isPending) ||
                  retryRemaining > 0
                }
                onClick={submitDraft}
              >
                {copy.onboarding.send}
              </Button>
            )}
            <Button
              block
              size="large"
              loading={manual.isPending}
              disabled={busy && !manual.isPending}
              onClick={runManual}
            >
              {copy.onboarding.manual}
            </Button>
          </div>
        </>
      ) : null}
    </div>
  );
}
