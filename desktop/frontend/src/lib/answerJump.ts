// How an answer's key points reach the surface that owns its scroller. The
// strip only emits; the owner performs the write through its own scroll writer,
// so the one-writer invariant stays intact (see check-single-scroll-writer).

import { createContext, useContext } from "react";

/** Aim the pane at an element inside one answer body. Returns false if it could not. */
export type AnswerJump = (element: HTMLElement) => boolean;

export const AnswerJumpContext = createContext<AnswerJump | null>(null);

export function useAnswerJump(): AnswerJump | null {
  return useContext(AnswerJumpContext);
}
