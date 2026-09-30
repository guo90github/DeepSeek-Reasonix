import { useCallback } from "react";
import { app } from "./bridge";
import type { RecapSkillDraft, RecapSkillSource } from "./types";

// The recap's skill actions live here because the controller that composes them is
// already thousands of lines past its ceiling: adding a capability there must not
// mean growing that file.
export function useRecapSkillActions() {
  const draftRecapSkill = useCallback(async (kind: string, body: string): Promise<RecapSkillDraft> => app.DraftRecapSkill(kind, body), []);
  const draftRecapTopicSkill = useCallback(async (sources: RecapSkillSource[]): Promise<RecapSkillDraft> => app.DraftRecapTopicSkill(sources), []);
  return { draftRecapSkill, draftRecapTopicSkill };
}
