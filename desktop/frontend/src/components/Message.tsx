import { createContext, lazy, memo, Suspense, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent, KeyboardEvent as ReactKeyboardEvent } from "react";
import { BrainCircuit, ChevronDown, FileText, Folder, GitBranch, Image, MessageSquare, Pencil, RotateCcw, ScrollText } from "lucide-react";
import { Markdown } from "./Markdown";
import { CopyButton } from "./CopyButton";
import { ComposerContextCard } from "./ComposerContextCard";
import { formatAttachmentRefForDisplay, formatAttachmentRefForSubmit, parseAttachmentRefsForDisplay, sortDisplayAttachments } from "../lib/attachmentDisplay";
import type { DisplayAttachment } from "../lib/attachmentDisplay";
import { app } from "../lib/bridge";
import { replaySubmitTextPreservingSelectedContext } from "../lib/editReplay";
import { useT } from "../lib/i18n";
import { ImageViewer } from "./ImageViewer";
import { Tooltip } from "./Tooltip";
import { useWorkProcessPresentation } from "../lib/sessionExperience";
import { stripMemoryCompilerExecution } from "../lib/memoryCompilerDisplay";
import { invocationSegmentsFromMessage, type InvocationMetadataMap } from "../lib/invocationDisplay";
import { messageActionLabelKey, type MessageActionScope } from "../lib/messageActions";
import type { Item } from "../lib/useController";
import type { CheckpointMeta } from "../lib/types";
import { InvocationBadge } from "./InvocationBadge";
import { AnswerKeyPoints } from "./AnswerKeyPoints";
import { EMPTY_ANSWER_MARKS, collectAnswerKeyPoints, sameAnswerKeyPoints, type AnswerKeyPoint, type AnswerMarkSummary } from "../lib/answerKeyPoints";
import { useAnswerJump } from "../lib/answerJump";
import { CodeViewer } from "./CodeViewer";
import { formatSelectionLabels, languageFor, parseSelectedTextContext, stripSelectionLabels } from "../lib/selectedTextContext";
import { imSourceLabel, parseImSourceMessage } from "../lib/imSourceMessage";
import { parsePastedBlocks, parseSelectedTextBlocks, type PastedBlockInfo, type SelectedTextBlockInfo } from "../lib/messagePasteBlocks";
const AuditInlineCard = lazy(() => import("./AuditInlineCard").then((module) => ({ default: module.AuditInlineCard })));
// The jump says "here": the landed mark keeps its ring until the reader moves
// on (a gesture), not for a fixed blink.
const LANDED_MARK_CLASS = "md--landed";

const AssistantReasoningPanel = lazy(() => import("./AssistantReasoningPanel").then((module) => ({ default: module.AssistantReasoningPanel })));
const MemoryCitations = lazy(() => import("./MemoryCitations").then((module) => ({ default: module.MemoryCitations })));
const SearchSourcesPanel = lazy(() => import("./SearchSourcesPanel").then((module) => ({ default: module.SearchSourcesPanel }))); type AssistantItem = Extract<Item, { kind: "assistant" }>;
export type TurnActionMenu = "summary" | "rewind" | "fork";
export const InvocationMetadataContext = createContext<InvocationMetadataMap>({});
function attachmentIcon(kind: "image" | "file" | "folder") {
  if (kind === "image") return <Image size={15} />;
  if (kind === "folder") return <Folder size={15} />;
  return <FileText size={15} />;
}

function mergeDisplayAttachments(existing: DisplayAttachment[], incoming: DisplayAttachment[]): DisplayAttachment[] {
  if (incoming.length === 0) return existing;
  const seen = new Set(existing.map((attachment) => attachment.path));
  const merged = [...existing];
  for (const attachment of incoming) {
    if (seen.has(attachment.path)) continue;
    seen.add(attachment.path);
    merged.push(attachment);
  }
  return merged;
}

function messageDate(value?: number): Date {
  return new Date(typeof value === "number" && Number.isFinite(value) && value > 0 ? value : Date.now());
}

function formatMessageTime(date: Date): string {
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  return `${hours}:${minutes}`;
}

export function UserMessage({
  text,
  submitText,
  failed,
  turn,
  anchorId,
  id,
  createdAt,
  onEdit,
  editDisabled = false,
}: {
  text: string;
  submitText?: string;
  failed?: boolean;
  turn?: number;
  anchorId?: string;
  id?: string;
  createdAt?: number;
  onEdit?: (turn: number, displayText: string, submitText?: string) => boolean | void | Promise<boolean | void>;
  editDisabled?: boolean;
}) {
  const t = useT();
  const invocationMetadata = useContext(InvocationMetadataContext);
  const imSource = parseImSourceMessage(text);
  const actionText = stripMemoryCompilerExecution(imSource?.text ?? text);
  const hasMemoryCompiler = Boolean(submitText?.includes("<memory-compiler-execution>"));
  const selectedTextEntries = useMemo(() => parseSelectedTextContext(submitText), [submitText]);
  const editableActionText = stripSelectionLabels(actionText, selectedTextEntries);
  const { text: editableDisplayText, attachments } = parseAttachmentRefsForDisplay(editableActionText);
  const selectionLabels = formatSelectionLabels(selectedTextEntries);
  const displayText = [editableDisplayText, selectionLabels].filter(Boolean).join(editableDisplayText && selectionLabels ? " " : "");
  const invocationSegments = imSource ? [] : invocationSegmentsFromMessage(displayText, submitText, invocationMetadata);
  const hasInvocationSegments = invocationSegments.some((segment) => segment.type === "invocation");
  const orderedAttachments = sortDisplayAttachments(attachments);
  const sourceLabel = imSource ? imSourceLabel(imSource, t) : "";
  const sentAt = createdAt === undefined ? null : messageDate(createdAt);
  const canEdit = turn !== undefined && onEdit !== undefined && !editDisabled;
  const [editing, setEditing] = useState(false);
  const [draftText, setDraftText] = useState(editableDisplayText);
  const [draftAttachments, setDraftAttachments] = useState<DisplayAttachment[]>(attachments);
  const [editSubmitting, setEditSubmitting] = useState(false);
  const editRef = useRef<HTMLTextAreaElement>(null);
  const [imagePreviews, setImagePreviews] = useState<Record<string, string>>({});
  const [imageViewer, setImageViewer] = useState<{ open: boolean; url: string; name: string }>({ open: false, url: "", name: "" });
  const openImageViewer = useCallback(async (path: string, name: string) => {
    let url = imagePreviews[path];
    if (!url) {
      try {
        url = await app.AttachmentDataURL(path);
        setImagePreviews((prev) => (prev[path] ? prev : { ...prev, [path]: url }));
      } catch {
        return;
      }
    }
    setImageViewer({ open: true, url, name });
  }, [imagePreviews]);

  const closeImageViewer = useCallback(() => {
    setImageViewer((prev) => (prev.open ? { ...prev, open: false } : prev));
  }, []);

  const pasteBlocks = useMemo(() => parsePastedBlocks(displayText, submitText), [displayText, submitText]);
  const selectedTextBlocks = useMemo(() => parseSelectedTextBlocks(displayText, submitText), [displayText, submitText]);
  const [expandedBlockKeys, setExpandedBlockKeys] = useState<Record<string, boolean>>({});

  type DisplaySegment =
    | { type: "text"; content: string }
    | { type: "block"; key: string; block: PastedBlockInfo; kind: "paste" }
    | { type: "block"; key: string; block: SelectedTextBlockInfo; kind: "chat" | "code" | "terminal" };

  const displaySegments = useMemo((): DisplaySegment[] => {
    if (pasteBlocks.length === 0 && selectedTextBlocks.length === 0) return [{ type: "text", content: displayText }];
    const segments: DisplaySegment[] = [];
    const ordered: Array<
      | { block: PastedBlockInfo; start: number; end: number; kind: "paste" }
      | { block: SelectedTextBlockInfo; start: number; end: number; kind: "chat" | "code" | "terminal" }
    > = [
      ...pasteBlocks.map((block) => {
        const start = displayText.indexOf(block.label);
        return { block, start, end: start + block.label.length, kind: "paste" as const };
      }),
      ...selectedTextBlocks.map((block) => ({ block, start: block.start, end: block.end, kind: block.kind })),
    ].filter((block) => block.start >= 0).sort((a, b) => a.start - b.start);
    let cursor = 0;
    for (const item of ordered) {
      if (item.start < cursor) continue;
      // Text before the label: strip the trailing newline that separated the
      // label from the preceding line so the card sits tight against the text.
      if (item.start > cursor) {
        let before = displayText.slice(cursor, item.start);
        before = before.replace(/\n$/, "");
        if (before) segments.push({ type: "text", content: before });
      }
      const key = `${item.kind}:${item.start}:${item.block.label}`;
      if (item.kind === "paste") {
        segments.push({ type: "block", key, block: item.block, kind: item.kind });
      } else {
        segments.push({ type: "block", key, block: item.block, kind: item.kind });
      }
      cursor = item.end;
    }
    // Strip the leading newline that followed the label.
    const remaining = displayText.slice(cursor).replace(/^\n/, "");
    if (remaining.trim()) segments.push({ type: "text", content: remaining });
    return segments.length > 0 ? segments : [{ type: "text", content: displayText }];
  }, [displayText, pasteBlocks, selectedTextBlocks]);

  const toggleBlockExpand = (key: string) => {
    setExpandedBlockKeys((prev) => ({
      ...prev,
      [key]: !prev[key],
    }));
  };
  const orderedDraftAttachments = sortDisplayAttachments(draftAttachments);
  const imagePreviewKey = orderedAttachments
    .concat(orderedDraftAttachments)
    .filter((attachment) => attachment.kind === "image" && attachment.source === "attachment")
    .map((attachment) => attachment.path)
    .join("\n");

  useEffect(() => {
    if (editing) return;
    const parsed = parseAttachmentRefsForDisplay(editableActionText);
    setDraftText(parsed.text);
    setDraftAttachments(parsed.attachments);
  }, [editableActionText, editing]);

  useEffect(() => {
    if (!editing) return;
    requestAnimationFrame(() => {
      const node = editRef.current;
      if (!node) return;
      node.focus();
      node.selectionStart = node.selectionEnd = node.value.length;
    });
  }, [editing]);

  const startEdit = () => {
    if (!canEdit) return;
    const parsed = parseAttachmentRefsForDisplay(editableActionText);
    setDraftText(parsed.text);
    setDraftAttachments(parsed.attachments);
    setEditing(true);
  };

  const cancelEdit = () => {
    const parsed = parseAttachmentRefsForDisplay(editableActionText);
    setDraftText(parsed.text);
    setDraftAttachments(parsed.attachments);
    setEditing(false);
  };

  const updateDraftText = (value: string) => {
    const parsed = parseAttachmentRefsForDisplay(value);
    if (parsed.attachments.length > 0) {
      setDraftText(parsed.text);
      setDraftAttachments((prev) => mergeDisplayAttachments(prev, parsed.attachments));
      return;
    }
    setDraftText(value);
  };

  const removeDraftAttachment = (path: string) => {
    setDraftAttachments((prev) => prev.filter((attachment) => attachment.path !== path));
  };

  const submitEdit = async (event?: FormEvent) => {
    event?.preventDefault();
    if (!canEdit || editSubmitting) return;
    const parsedDraft = parseAttachmentRefsForDisplay(draftText);
    const nextAttachments = sortDisplayAttachments(mergeDisplayAttachments(draftAttachments, parsedDraft.attachments));
    const bodyText = parsedDraft.text.trim();
    const displayRefs = nextAttachments.map(formatAttachmentRefForDisplay).join(" ");
    const submitRefs = nextAttachments.map(formatAttachmentRefForSubmit).join(" ");
    const nextEditable = [bodyText, displayRefs].filter(Boolean).join(bodyText && displayRefs ? " " : "");
    const next = [nextEditable, selectionLabels].filter(Boolean).join(nextEditable && selectionLabels ? " " : "");
    const fallbackSubmit = [bodyText, submitRefs].filter(Boolean).join(bodyText && submitRefs ? " " : "");
    const submit = replaySubmitTextPreservingSelectedContext(submitText, editableActionText, nextEditable, fallbackSubmit);
    if (!next) return;
    setEditSubmitting(true);
    try {
      const ok = await onEdit?.(turn as number, next, submit);
      if (ok !== false) setEditing(false);
    } finally {
      setEditSubmitting(false);
    }
  };

  const onEditKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      cancelEdit();
      return;
    }
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
      void submitEdit();
    }
  };

  useEffect(() => {
    const paths = imagePreviewKey ? imagePreviewKey.split("\n") : [];
    if (paths.length === 0) return;
    let cancelled = false;
    for (const path of paths) {
      if (imagePreviews[path]) continue;
      app.AttachmentDataURL(path)
        .then((url) => {
          if (cancelled) return;
          setImagePreviews((prev) => (prev[path] ? prev : { ...prev, [path]: url }));
        })
        .catch(() => {});
    }
    return () => {
      cancelled = true;
    };
  }, [imagePreviewKey]);
  return (
    <div
      className={`msg msg--user${imSource ? " msg--im-source" : ""}${failed ? " msg--user-failed" : ""}`}
      id={anchorId}
      data-question-anchor={anchorId}
      data-turn={turn}
      data-im-source={imSource?.provider || undefined}
      data-history-restore={id && id.startsWith("h") ? "" : undefined}
      data-entrance={id || undefined}
    >
      <div className={`msg__body${editing ? " msg__body--editing" : ""}`} data-transcript-selectable="message">
        {editing ? (
          <form className="msg-edit" onSubmit={(event) => void submitEdit(event)}>
            {orderedDraftAttachments.length > 0 && (
              <div className="msg-edit__attachments composer-context" aria-label={t("composer.contextItems")}>
                {orderedDraftAttachments.map((attachment) => {
                  const imagePreview = attachment.kind === "image" ? imagePreviews[attachment.path] : undefined;
                  const imageOnly = Boolean(imagePreview) && orderedDraftAttachments.every((item) => item.kind === "image" && imagePreviews[item.path]);
                  return (
                    <ComposerContextCard
                      key={attachment.path}
                      variant={attachment.source === "workspace" ? "workspace" : "attachment"}
                      tooltipLabel={imagePreview ? `${t("imageViewer.clickToPreview")} — ${attachment.path}` : attachment.source === "workspace" ? formatAttachmentRefForSubmit(attachment) : attachment.path}
                      removeLabel={attachment.source === "workspace" ? t("composer.removeReference") : t("composer.removeImage")}
                      removeDisabled={editSubmitting}
                      onRemove={() => removeDraftAttachment(attachment.path)}
                      previewUrl={imagePreview}
                      onImageClick={imagePreview ? () => openImageViewer(attachment.path, attachment.name) : undefined}
                      imageOnly={imageOnly}
                      folder={attachment.kind === "folder"}
                      label={attachment.kind === "folder" ? `${attachment.name}/` : attachment.name}
                      name={attachment.name}
                      meta={attachment.ext || t("msg.fileAttachment")}
                      icon={attachment.kind === "image" ? <Image size={20} /> : undefined}
                    />
                  );
                })}
              </div>
            )}
            <textarea
              ref={editRef}
              className="msg-edit__input"
              value={draftText}
              rows={Math.max(2, Math.min(8, draftText.split(/\r?\n/).length))}
              aria-label={t("common.edit")}
              disabled={editSubmitting}
              onChange={(event) => updateDraftText(event.target.value)}
              onKeyDown={onEditKeyDown}
            />
            <div className="msg-edit__actions">
              <button className="msg-edit__btn" type="button" disabled={editSubmitting} onClick={cancelEdit}>
                {t("common.cancel")}
              </button>
              <button className="msg-edit__btn msg-edit__btn--primary" type="submit" disabled={editSubmitting || (draftText.trim() === "" && draftAttachments.length === 0 && selectedTextEntries.length === 0)}>
                {t("msg.editSend")}
              </button>
            </div>
          </form>
        ) : imSource ? (
          <div className="im-source-card">
            <div className="im-source-card__head" data-transcript-selection-ignore>
              <MessageSquare size={14} />
              <span>{t("msg.fromIm", { source: sourceLabel })}</span>
            </div>
            {displayText && <div className="im-source-card__text">{displayText}</div>}
            {(imSource.sender || imSource.chat) && (
              <div className="im-source-card__meta" data-transcript-selection-ignore>
                {imSource.sender && <span>{t("msg.imSender", { id: imSource.sender })}</span>}
                {imSource.chat && <span>{imSource.chat}</span>}
              </div>
            )}
          </div>
        ) : (
          <>
            {hasInvocationSegments && pasteBlocks.length === 0 && selectedTextBlocks.length === 0 ? (
              <div className="msg__text msg__rich-text">
                {invocationSegments.map((segment, index) => segment.type === "text"
                  ? <span key={`text:${segment.start}:${index}`}>{segment.content}</span>
                  : (
                    <InvocationBadge
                      key={`invocation:${segment.invocation.name}:${segment.offset}:${index}`}
                      invocation={segment.invocation}
                      kind={segment.invocation.kind}
                      variant="message"
                    />
                  ))}
              </div>
            ) : displaySegments.map((seg, i) => {
              if (seg.type === "text") {
                return seg.content ? <div className="msg__text" key={`s${i}`}>{seg.content}</div> : null;
              }
              const expanded = Boolean(expandedBlockKeys[seg.key]);
              return (
                <div className="msg-pasted" key={seg.key}>
                  <div className="msg-pasted-block">
                    <div className="msg-pasted-head" data-transcript-selection-ignore>
                      {seg.kind === "code" ? <FileText size={15} /> : <MessageSquare size={15} />}
                      <span className="msg-pasted-label">{seg.block.label}</span>
                      <div className="msg-pasted-actions">
                        <Tooltip label={t(expanded ? "msg.pastedCollapseTooltip" : "msg.pastedExpandTooltip")}>
                          <button type="button" onClick={() => toggleBlockExpand(seg.key)}>
                            {expanded ? t("common.collapse") : t("composer.pastedExpand")}
                          </button>
                        </Tooltip>
                      </div>
                    </div>
                    {expanded && (
                      <div className="msg-pasted-expanded">
                        {seg.kind === "chat"
                          ? <Markdown text={seg.block.content} />
                          : seg.kind === "code" || seg.kind === "terminal"
                            ? <CodeViewer value={seg.block.content} language={seg.kind === "terminal" ? "console" : languageFor(seg.block.path ?? "")} maxHeight={360} />
                            : seg.block.content}
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </>
        )}
        {failed && <div className="msg__send-failed" data-transcript-selection-ignore>{t("msg.sendFailed")}</div>}
        {orderedAttachments.length > 0 && (
          <div className="msg-attachments" aria-label={t("msg.attachments")} data-transcript-selection-ignore>
            {orderedAttachments.map((attachment, index) => {
              const isImage = attachment.kind === "image";
              const el = (
                <div
                  className={`msg-attachment msg-attachment--${attachment.kind}`}
                  key={isImage ? undefined : `${attachment.path}:${index}`}
                  title={isImage ? undefined : attachment.path}
                  onClick={isImage ? () => openImageViewer(attachment.path, attachment.name) : undefined}
                  role={isImage ? "button" : undefined}
                  tabIndex={isImage ? 0 : undefined}
                  onKeyDown={isImage ? (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); openImageViewer(attachment.path, attachment.name); } } : undefined}
                >
                  <span className={`msg-attachment__icon msg-attachment__icon--${attachment.kind}`} aria-hidden="true">
                    {isImage && imagePreviews[attachment.path] ? <img src={imagePreviews[attachment.path]} alt="" draggable={false} /> : attachmentIcon(attachment.kind)}
                  </span>
                  <span className="msg-attachment__main">
                    <span className="msg-attachment__name">{attachment.name}</span>
                    <span className="msg-attachment__meta">
                      {attachment.kind === "folder"
                        ? t("msg.folderReference")
                        : `${attachment.ext || t("msg.fileAttachment")} · ${attachment.source === "workspace" ? t("msg.workspaceReference") : attachment.kind === "image" ? t("msg.imageAttachment") : t("msg.fileAttachment")}`}
                    </span>
                  </span>
                </div>
              );
              if (isImage) {
                return (
                  <Tooltip key={`${attachment.path}:${index}`} label={t("imageViewer.clickToPreview")} block>
                    {el}
                  </Tooltip>
                );
              }
              return el;
            })}
            <ImageViewer
              open={imageViewer.open}
              imageUrl={imageViewer.url}
              imageName={imageViewer.name}
              onClose={closeImageViewer}
            />
          </div>
        )}
      </div>
      {!editing && (
        <div className="msg-meta" role="group" aria-label={t("rewind.label")}>
          {sentAt && (
            <time className="msg-meta__time" dateTime={sentAt.toISOString()} title={sentAt.toLocaleString()}>
              {formatMessageTime(sentAt)}
            </time>
          )}
          {hasMemoryCompiler && (
            <span className="msg-meta__indicator" title={t("msg.memoryCompilerApplied")} aria-hidden="true">
              <BrainCircuit size={14} />
            </span>
          )}
          <CopyButton text={actionText} label={t("msg.copy")} showInlineLabel={false} className="msg-meta__btn msg-meta__copy" />
          {onEdit && (
            <button
              className="msg-meta__btn"
              type="button"
              aria-label={t("common.edit")}
              title={t("common.edit")}
              disabled={!canEdit}
              onClick={startEdit}
            >
              <Pencil size={14} />
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export function TurnActions({
  text,
  turn,
  openMenu,
  onOpenMenu,
  onRewind,
  checkpoint,
  actionPending = false,
  rewindDisabled = false,
  hoverMenus = false,
  isLastTurn = false,
}: {
  text: string;
  turn?: number;
  openMenu?: TurnActionMenu | null;
  onOpenMenu?: (menu: TurnActionMenu | null) => void;
  onRewind?: (turn: number, scope: MessageActionScope) => void;
  checkpoint?: CheckpointMeta;
  actionPending?: boolean;
  rewindDisabled?: boolean;
  hoverMenus?: boolean;
  /** true when this is the last user turn — disables "summarize after" */
  isLastTurn?: boolean;
}) {
  const t = useT();
  const [confirmScope, setConfirmScope] = useState<MessageActionScope | null>(null);
  const canAct = onRewind != null && turn != null;
  const actionDisabledReason = (scope: string): string => {
    if (rewindDisabled || actionPending) return t("rewind.disabledRunning");
    if (!checkpoint) return t("rewind.disabledNoCheckpoint");
    if ((scope === "fork" || scope === "fork-worktree" || scope === "summ-from" || scope === "conversation") && !checkpoint.canConversation) {
      return t("rewind.disabledNoBoundary");
    }
    if (scope === "summ-from" && isLastTurn) {
      return t("rewind.disabledNoLater");
    }
    if (scope === "summ-upto") {
      if (!checkpoint.canConversation) return t("rewind.disabledNoBoundary");
      if ((turn ?? 0) <= 0) return t("rewind.disabledNoEarlier");
    }
    if (scope === "code" && !checkpoint.canCode) return t("rewind.disabledNoCode");
    if (scope === "both") {
      if (!checkpoint.canConversation) return t("rewind.disabledNoBoundary");
      if (!checkpoint.canCode) return t("rewind.disabledNoCode");
    }
    return "";
  };
  const actionLabel = (scope: MessageActionScope): string => t(messageActionLabelKey(scope, confirmScope === scope));
  const actionMeta = (scope: MessageActionScope): string => {
    const total = checkpoint?.fileCount ?? checkpoint?.files?.length ?? 0;
    if ((scope === "code" || scope === "both") && total > 0) {
      const turnCount = checkpoint?.turnFileCount ?? 0;
      if (turnCount > 0 && turnCount < total) {
        return `${t("rewind.filesChanged", { count: total })} (${t("rewind.turnFiles", { count: turnCount })})`;
      }
      return t("rewind.filesChanged", { count: total });
    }
    return "";
  };
  const actionTooltipLabel = (scope: MessageActionScope) => {
    const reason = actionDisabledReason(scope);
    if (reason) return <span>{reason}</span>;
    const files = checkpoint?.files ?? [];
    const total = checkpoint?.fileCount ?? files.length;
    if ((scope === "code" || scope === "both") && total > 0) {
      const hidden = Math.max(0, total - files.length);
      return (
        <div className="rewind__files-tooltip">
          {files.map((file) => (
            <div key={file}>{file.split(/[/\\]/).pop() || file}</div>
          ))}
          {hidden > 0 && <div>+{hidden}</div>}
        </div>
      );
    }
    return undefined;
  };
  const runAction = (scope: MessageActionScope) => {
    setConfirmScope(null);
    onOpenMenu?.(null);
    onRewind?.(turn as number, scope);
  };
  const selectRewind = (scope: MessageActionScope) => {
    if (actionDisabledReason(scope)) return;
    if (confirmScope !== scope) {
      setConfirmScope(scope);
      return;
    }
    runAction(scope);
  };
  const renderAction = (scope: MessageActionScope, danger = false) => {
    const disabledReason = actionDisabledReason(scope);
    const meta = actionMeta(scope);
    const tipLabel = actionTooltipLabel(scope);
    const button = (
      <button
        className={[
          "rewind__menu-item",
          danger ? "rewind__menu-danger" : "",
          confirmScope === scope ? "rewind__menu-confirm" : "",
        ].filter(Boolean).join(" ")}
        type="button"
        disabled={Boolean(disabledReason)}
        {...(tipLabel ? {} : { title: disabledReason || undefined })}
        onClick={() => selectRewind(scope)}
      >
        <span>{actionLabel(scope)}</span>
        {meta && <span className="rewind__menu-meta">{meta}</span>}
      </button>
    );
    return tipLabel ? <Tooltip key={scope} label={tipLabel} side="top" block fill>{button}</Tooltip> : button;
  };
  const forkDisabledReason = canAct ? actionDisabledReason("fork") : "";
  const toggleMenu = (menu: TurnActionMenu) => {
    setConfirmScope(null);
    onOpenMenu?.(openMenu === menu ? null : menu);
  };
  const openHoverMenu = (menu: TurnActionMenu) => {
    if (!hoverMenus || openMenu === menu) return;
    setConfirmScope(null);
    onOpenMenu?.(menu);
  };
  return (
    <div className={`turn-actions${openMenu ? " turn-actions--open" : ""}${hoverMenus ? " turn-actions--hover-menu" : ""}`}>
      {text.trim() && <CopyButton text={text} label={t("msg.copy")} />}
      {canAct && (
        <>
          <div
            className={`turn-actions__group${openMenu === "fork" ? " turn-actions__group--open" : ""}`}
            onMouseEnter={() => openHoverMenu("fork")}
          >
            <button
              className={`turn-actions__btn${confirmScope === "fork" || confirmScope === "fork-worktree" ? " turn-actions__btn--confirm" : ""}`}
              type="button"
              disabled={Boolean(forkDisabledReason)}
              aria-haspopup="menu"
              aria-expanded={openMenu === "fork"}
              title={forkDisabledReason || t("rewind.forkTooltip")}
              onClick={() => toggleMenu("fork")}
            >
              <GitBranch size={13} />
              <span className="turn-actions__label-inline">
                <span>{confirmScope === "fork-worktree" ? actionLabel("fork-worktree") : (confirmScope === "fork" ? actionLabel("fork") : t("rewind.fork"))}</span>
                <ChevronDown size={12} />
              </span>
            </button>
            {openMenu === "fork" && (
              <div className="rewind__menu turn-actions__menu" role="menu">
                {renderAction("fork-worktree")}
                {renderAction("fork")}
              </div>
            )}
          </div>
          <div
            className={`turn-actions__group${openMenu === "summary" ? " turn-actions__group--open" : ""}`}
            onMouseEnter={() => openHoverMenu("summary")}
          >
            <button
              className="turn-actions__btn"
              type="button"
              aria-haspopup="menu"
              aria-expanded={openMenu === "summary"}
              onClick={() => toggleMenu("summary")}
            >
              <ScrollText size={13} />
              <span className="turn-actions__label-inline">
                <span>{t("turnActions.summary")}</span>
                <ChevronDown size={12} />
              </span>
            </button>
            {openMenu === "summary" && (
              <div className="rewind__menu turn-actions__menu" role="menu">
                {rewindDisabled && <div className="rewind__menu-hint">{t("rewind.disabledRunning")}</div>}
                {!rewindDisabled && !checkpoint && <div className="rewind__menu-hint">{t("rewind.disabledNoCheckpoint")}</div>}
                {renderAction("summ-from")}
                {renderAction("summ-upto")}
              </div>
            )}
          </div>
          <div
            className={`turn-actions__group${openMenu === "rewind" ? " turn-actions__group--open" : ""}`}
            onMouseEnter={() => openHoverMenu("rewind")}
          >
            <button
              className="turn-actions__btn"
              type="button"
              aria-haspopup="menu"
              aria-expanded={openMenu === "rewind"}
              onClick={() => toggleMenu("rewind")}
            >
              <RotateCcw size={13} />
              <span className="turn-actions__label-inline">
                <span>{t("turnActions.rewind")}</span>
                <ChevronDown size={12} />
              </span>
            </button>
            {openMenu === "rewind" && (
              <div className="rewind__menu turn-actions__menu" role="menu">
                {rewindDisabled && <div className="rewind__menu-hint">{t("rewind.disabledRunning")}</div>}
                {!rewindDisabled && !checkpoint && <div className="rewind__menu-hint">{t("rewind.disabledNoCheckpoint")}</div>}
                {renderAction("conversation")}
                {renderAction("code")}
                {renderAction("both", true)}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}

export const AssistantMessage = memo(function AssistantMessage({
  item,
  defaultExpanded = false,
  expandWhileStreaming = false,
  creationMode = false,
}: {
  item: AssistantItem;
  defaultExpanded?: boolean;
  /** false in compact mode: completed steps fold away, so auto-open + fold reads as flicker. */
  expandWhileStreaming?: boolean;
  creationMode?: boolean;
}) {
  const presentation = useWorkProcessPresentation();
  const hasText = item.streaming || item.text.trim() !== "";
  const hasFootnotes = Boolean(item.searchSources?.length);
  const processOnly = Boolean(item.reasoning) && !hasText && !hasFootnotes;
  const processWithText = Boolean(item.reasoning) && (hasText || hasFootnotes);
  const reasoningFallback = <div className="reasoning reasoning--loading" data-expanded={defaultExpanded || presentation.keepExpandedAfterCompletion || (item.streaming && (presentation.showWhileRunning || expandWhileStreaming)) ? "" : undefined} aria-hidden />;
  // The key-point strip reads the emphasis marks out of the answer body. All
  // three markdown paths swap their DOM in without a React signal — the worker
  // owns the settled parse — so the marks are observed, not passed up.
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const [keyPoints, setKeyPoints] = useState<AnswerMarkSummary>(EMPTY_ANSWER_MARKS);
  const answerJump = useAnswerJump();
  const [landed, setLanded] = useState<HTMLElement | null>(null);
  // A point names the mark by address, so the target is looked up inside this
  // answer's own body — never globally (another answer also has a "0"). No
  // mark means no jump: the strip never guesses a destination.
  const jumpToPoint = useCallback((point: AnswerKeyPoint) => {
    if (!answerJump) return;
    const target = bodyRef.current?.querySelector(`[data-md-point="${point.ordinal}"]`) as HTMLElement | null;
    if (!target) return;
    setLanded(target);
    answerJump(target);
  }, [answerJump]);
  // The ring stays until the reader moves on: a gesture, not a timer, is what
  // says "I am done looking at this". The jump's own scroll is programmatic and
  // emits no gesture, so it cannot clear itself. One landed mark at a time.
  useEffect(() => {
    if (!landed) return;
    const doc = landed.ownerDocument;
    for (const other of doc.querySelectorAll(`.${LANDED_MARK_CLASS}`)) {
      if (other !== landed) other.classList.remove(LANDED_MARK_CLASS);
    }
    landed.classList.add(LANDED_MARK_CLASS);
    const view = doc.defaultView;
    const clear = () => setLanded(null);
    const gestures: (keyof WindowEventMap)[] = ["wheel", "pointerdown", "touchstart", "keydown"];
    for (const name of gestures) view?.addEventListener(name, clear, { capture: true, passive: true });
    return () => {
      landed.classList.remove(LANDED_MARK_CLASS);
      for (const name of gestures) view?.removeEventListener(name, clear, { capture: true });
    };
  }, [landed]);
  useEffect(() => {
    if (item.streaming) {
      setKeyPoints((current) => (current === EMPTY_ANSWER_MARKS ? current : EMPTY_ANSWER_MARKS));
      return;
    }
    const root = bodyRef.current;
    if (!root) return;
    // The scheduler and the observer come from the document that owns the row:
    // jsdom-based suites expose them on the window, not as globals.
    const view = root.ownerDocument?.defaultView;
    const defer: (run: () => void) => number = typeof requestAnimationFrame === "function"
      ? (run) => requestAnimationFrame(run)
      : (run) => view?.setTimeout(run, 0) ?? 0;
    const drop: (handle: number) => void = typeof cancelAnimationFrame === "function"
      ? (handle) => cancelAnimationFrame(handle)
      : (handle) => view?.clearTimeout(handle);
    let frame = 0;
    const read = () => {
      frame = 0;
      const next = collectAnswerKeyPoints(root);
      setKeyPoints((current) => (sameAnswerKeyPoints(current, next) ? current : next));
    };
    const schedule = () => {
      if (frame === 0) frame = defer(read);
    };
    read();
    const observer = view?.MutationObserver ? new view.MutationObserver(schedule) : null;
    observer?.observe(root, { childList: true, subtree: true, attributes: true, attributeFilter: ["class"] });
    return () => {
      observer?.disconnect();
      if (frame !== 0) drop(frame);
    };
  }, [item.id, item.streaming, item.text]);
  return (
    <div className={`msg msg--assistant${processOnly ? " msg--process-only" : ""}${processWithText ? " msg--process-with-text" : ""}`} data-history-restore={item.id.startsWith("h") ? "" : undefined} data-entrance={item.id}>
      {item.reasoning && (
        <Suspense fallback={reasoningFallback}>
          <AssistantReasoningPanel item={item} defaultExpanded={defaultExpanded} expandWhileStreaming={expandWhileStreaming} />
        </Suspense>
      )}
      {item.reasoning && (
        <Suspense fallback={null}>
          <AuditInlineCard reasoning={item.reasoning} />
        </Suspense>
      )}
      {keyPoints.points.length > 1 && (
        <AnswerKeyPoints points={keyPoints.points} total={keyPoints.total} onJump={answerJump ? jumpToPoint : undefined} />
      )}
      {(hasText || hasFootnotes) && (
        <div className="msg__body" ref={bodyRef} data-transcript-selectable="message">
          {hasText && (
            <Markdown
              text={item.text}
              plainStatusBlocks={creationMode}
              streaming={item.streaming}
              cacheKey={item.id}
              wasStreamed={item.wasStreamed}
            />
          )}
          <Suspense fallback={null}><SearchSourcesPanel sources={item.searchSources} /></Suspense>
        </div>
      )}
      {Boolean(item.memoryCitations?.length) && <Suspense fallback={null}><MemoryCitations citations={item.memoryCitations} /></Suspense>}
    </div>
  );
});
