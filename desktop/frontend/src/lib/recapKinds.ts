// The distilled-note kinds this build can label. A kind it does not know is shown as it
// arrived rather than dropped, so the mapping is deliberately partial.
export type RecapKindKey =
  | "history.recapKindFact"
  | "history.recapKindRootCause"
  | "history.recapKindRefuted"
  | "history.recapKindHandoff";

export function kindKey(kind: string): RecapKindKey | null {
  switch (kind) {
    case "fact": return "history.recapKindFact";
    case "root-cause": return "history.recapKindRootCause";
    case "refuted": return "history.recapKindRefuted";
    case "handoff": return "history.recapKindHandoff";
    default: return null;
  }
}
