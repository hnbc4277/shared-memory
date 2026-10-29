/** Optional GitHub (or docs site) base, e.g. https://github.com/org/repo/blob/main */
const repoBase = (import.meta.env.VITE_REPO_DOCS_BASE as string | undefined)?.replace(/\/$/, "") ?? "";

function repoPath(path: string): string {
  if (repoBase) {
    return `${repoBase}/${path.replace(/^\//, "")}`;
  }
  return `/${path.replace(/^\//, "")}`;
}

export const DOC_LINKS = {
  installCursor: repoPath("docs/superpowers/specs/2026-09-30-shared-memory-design.md"),
  hermes: repoPath("adapters/hermes/README.md"),
  openclaw: repoPath("adapters/openclaw/README.md"),
  deepseek: repoPath("adapters/deepseek-harness/README.md"),
  doubao: repoPath("adapters/doubao/README.md"),
  agentsSpec: repoPath("docs/superpowers/specs/2026-10-12-agent-connections-design.md"),
} as const;
