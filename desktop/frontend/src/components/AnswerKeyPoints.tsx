// The answer's key points, lifted above the body so a long answer can be
// skimmed without reading it. Chrome, not content: it renders outside the
// selectable root, so copying the answer never picks these lines up twice.

import { useT } from "../lib/i18n";
import type { AnswerKeyPoint } from "../lib/answerKeyPoints";

export function AnswerKeyPoints({ points, total, onJump }: {
  points: readonly AnswerKeyPoint[];
  total: number;
  /** Absent until the owner of the answer's scroller can act on a jump. */
  onJump?: (point: AnswerKeyPoint) => void;
}) {
  const t = useT();
  if (points.length < 2) return null;
  const head = t("answer.pointsHead", { n: total });
  return (
    <div className="answer-points" role="note">
      <span className="answer-points__head">{head}</span>
      <ol className="answer-points__list">
        {points.map((point) => (
          <li key={point.ordinal} className={`answer-points__item answer-points__item--${point.kind}`}>
            <span className="answer-points__mark" aria-hidden="true" />
            {onJump ? (
              <button
                type="button"
                className="answer-points__text"
                data-md-point-target={point.ordinal}
                title={point.text}
                onClick={() => onJump(point)}
              >
                {point.text}
              </button>
            ) : (
              <span className="answer-points__text" title={point.text}>{point.text}</span>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}
