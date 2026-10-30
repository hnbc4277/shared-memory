/** Optional GitHub (or docs site) base, e.g. https://github.com/org/repo/blob/main */
const repoBase = (import.meta.env.VITE_REPO_DOCS_BASE as string | undefined)?.replace(/\/$/, "") ?? "";

function repoPath(path: string): string {
  if (repoBase) {
    return `${repoBase}/${path.replace(/^\//, "")}`;
  }
  return `/${path.replace(/^\//, "")}`;
}

/** Console doc links — only paths that ship in the public tree / web/public. */
export const DOC_LINKS = {
  installCursor: repoPath("docs/公开/manual/CURSOR-STABLE-SETUP.md"),
  hermes: repoPath("adapters/hermes/README.md"),
  openclaw: repoPath("adapters/openclaw/README.md"),
  deepseek: repoPath("adapters/deepseek-harness/README.md"),
  doubao: repoPath("adapters/doubao/README.md"),
  agentsSpec: repoPath("docs/公开/design/SYSTEM-DESIGN.md"),
} as const;
