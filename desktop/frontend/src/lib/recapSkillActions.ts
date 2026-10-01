import { useCallback } from "react";
import { app } from "./bridge";
import type { RecapPreviewView, RecapSkillDraft, RecapSkillSource } from "./types";

// The recap's skill actions live here because the controller that composes them is
// already thousands of lines past its ceiling: adding a capability there must not
// mean growing that file.
export function useRecapSkillActions() {
  const draftRecapSkill = useCallback(async (kind: string, body: string): Promise<RecapSkillDraft> => app.DraftRecapSkill(kind, body), []);
  const draftRecapTopicSkill = useCallback(async (sources: RecapSkillSource[], markdown: string): Promise<RecapSkillDraft> => app.DraftRecapTopicSkill(sources, markdown), []);
  const previewRecapMemory = useCallback(async (source: RecapSkillSource): Promise<RecapPreviewView> => app.PreviewRecapMemory(source), []);
  const previewRecapSkill = useCallback(async (sources: RecapSkillSource[]): Promise<RecapPreviewView> => app.PreviewRecapSkill(sources), []);
  const recallRecordForSession = useCallback(async (sessionPath: string) => app.RecallRecordForSession(sessionPath), []);
  return { draftRecapSkill, draftRecapTopicSkill, previewRecapMemory, previewRecapSkill, recallRecordForSession };
}
