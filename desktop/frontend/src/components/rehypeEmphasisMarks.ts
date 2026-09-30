// Structural emphasis marks for the answer ladder. CSS cannot state these two
// facts: `:only-child`/`:first-child` ignore text nodes, so `**要点**：说明`
// looked like a fully bold paragraph. The mark is data; styles.css decides what
// each surface paints.

type HastNode = {
  type: string;
  tagName?: string;
  value?: string;
  properties?: Record<string, unknown>;
  children?: HastNode[];
};

export const LABEL_CLASS = "md-li--label";
export const CLAIM_CLASS = "md-p--claim";

function isBlankText(node: HastNode): boolean {
  return node.type === "text" && !/\S/.test(node.value ?? "");
}

function meaningfulChildren(node: HastNode): HastNode[] {
  return (node.children ?? []).filter((child) => !isBlankText(child));
}

function isStrong(node: HastNode | undefined): boolean {
  return node?.type === "element" && node.tagName === "strong";
}

function mark(node: HastNode, name: string): void {
  const current = node.properties?.className;
  const classes = Array.isArray(current) ? current.filter((item): item is string => typeof item === "string") : [];
  if (!classes.includes(name)) classes.push(name);
  node.properties ??= {};
  node.properties.className = classes;
}

/**
 * `md-li--label`: the bullet opens with the author's own bolded label — the
 * first thing after the marker, whether the list is tight or loose.
 * `md-p--claim`: the paragraph is bold end to end, with no other text in it.
 */
export function rehypeEmphasisMarks() {
  const visit = (node: HastNode): void => {
    if (node.type === "element" && node.tagName === "li") {
      const lead = meaningfulChildren(node)[0];
      const label = lead?.tagName === "p" ? meaningfulChildren(lead)[0] : lead;
      if (isStrong(label)) mark(node, LABEL_CLASS);
    }
    if (node.type === "element" && node.tagName === "p") {
      const only = meaningfulChildren(node);
      if (only.length === 1 && isStrong(only[0])) mark(node, CLAIM_CLASS);
    }
    for (const child of node.children ?? []) visit(child);
  };
  return (tree: unknown): void => {
    visit(tree as HastNode);
  };
}
