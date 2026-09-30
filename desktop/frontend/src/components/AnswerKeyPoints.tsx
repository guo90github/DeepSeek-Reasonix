// The answer's key points, lifted above the body so a long answer can be
// skimmed without reading it. Chrome, not content: it renders outside the
// selectable root, so copying the answer never picks these lines up twice.

import { useT } from "../lib/i18n";
import type { AnswerKeyPoint } from "../lib/answerKeyPoints";

export function AnswerKeyPoints({ points }: { points: readonly AnswerKeyPoint[] }) {
  const t = useT();
  if (points.length < 2) return null;
  return (
    <div className="answer-points" role="note" aria-label={t("answer.pointsHead", { n: points.length })}>
      <span className="answer-points__head" aria-hidden="true">{t("answer.pointsHead", { n: points.length })}</span>
      <ol className="answer-points__list">
        {points.map((point, index) => (
          <li key={`${index}-${point.text}`} className={`answer-points__item answer-points__item--${point.kind}`}>
            <span className="answer-points__mark" aria-hidden="true" />
            <span className="answer-points__text" title={point.text}>{point.text}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}
