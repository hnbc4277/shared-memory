import { API_BASE } from "../api/client";
import { CodeBlock, Modal } from "../ui";

const HOOK_BIN = "%USERPROFILE%\\.shared-memory\\smhook.exe";
const MCP_BIN = "%USERPROFILE%\\.shared-memory\\shared-memory-mcp.exe";
const HOOK_CMD = `cmd /c "set SHARED_MEMORY_HOOK_SURFACE=cursor&& ${HOOK_BIN}"`;

export function CursorSetupGuide({
  connectionId,
  apiKey,
  onClose,
}: {
  connectionId: string;
  apiKey?: string;
  onClose?: () => void;
}) {
  const key = apiKey && apiKey.length > 0 ? apiKey : "<专用密钥>";
  const cred = {
    api_url: API_BASE,
    api_key: key,
    connection_id: connectionId,
  };
  const hooks = {
    beforeSubmitPrompt: [{ name: "shared-memory", command: HOOK_CMD }],
    afterAgentResponse: [{ name: "shared-memory", command: HOOK_CMD }],
    subagentStop: [{ name: "shared-memory", command: HOOK_CMD }],
  };
  const mcp = {
    command: MCP_BIN,
    env: {
      SHARED_MEMORY_API_URL: API_BASE,
      SHARED_MEMORY_API_KEY: key,
      SHARED_MEMORY_AGENT: "cursor",
      SHARED_MEMORY_CONNECTION_ID: connectionId,
    },
  };
  const rule = `---
description: Shared Memory — recall at the start of non-trivial tasks
globs:
alwaysApply: true
---

# Shared Memory

每个非琐碎任务开始时：

1. 调用 memory_recall。query 用当前用户目标的一句话。
2. 把返回的 context 当作已知背景，不要让用户重复已经记住的偏好和决定。
3. 不要调用 memory_ingest。用户消息和助手回复已由钩子写入。
4. 不要在对话或工具参数里打印 API Key。
5. memory_recall 失败时继续任务，不要卡住。
`;

  return (
    <Modal title="Cursor 配置步骤" onClose={onClose ?? (() => {})} wide>
      <div id="cursor-setup-guide">
        <p className="small muted">
          写入靠钩子，不依赖模型是否调用工具。召回靠 MCP 的 memory_recall，模型仍可能漏调。
          不要再调用 memory_ingest。不要另开 shared-memory-daemon（127.0.0.1:18765），否则专用密钥不会生效。
        </p>
        <ol className="cursor-setup-steps">
          <li>
            <strong>推荐：一键脚本</strong>
            <p className="small muted">
              创建时勾选「专用 API 密钥」后，行内「下载配置脚本」会先提示敏感信息，确认后再下 zip。
              包内脚本<strong>已写死本连接的 connection_id 与专用密钥</strong>，双击 <code>.cmd</code> 即可安装，不会再让你填写这两项。
              请勿把 zip 发给无关人员。程序装到当前用户的 <code>%USERPROFILE%\.shared-memory\</code>。
            </p>
            <p className="small muted">
              若没有「下载配置脚本」按钮，说明创建时未签发专用密钥，或这是旧连接——请吊销后重新添加并勾选「专用 API 密钥」。
              下面是手工步骤对照。
            </p>
          </li>
          <li>
            <strong>保存凭证</strong>
            <p className="small muted">connection_id 可以随时在本页再看。专用 API 密钥只在创建时显示一次。</p>
            <CodeBlock label="connection_id">
              <pre>{connectionId}</pre>
            </CodeBlock>
            {apiKey ? (
              <CodeBlock label="专用 API 密钥（仅此次）">
                <pre>{apiKey}</pre>
              </CodeBlock>
            ) : (
              <p className="small muted">若密钥已丢失，吊销该连接后重新添加，并勾选「专用 API 密钥」。</p>
            )}
          </li>
          <li>
            <strong>下载程序并放到固定目录</strong>
            <p className="small muted">
              Windows 64 位。放到<strong>当前用户</strong>的 <code>%USERPROFILE%\.shared-memory\</code>
              （或你设的 <code>SHARED_MEMORY_HOME</code>），不要抄别人机器上的 <code>C:\Users\某用户\...</code>。
              手工改 mcp.json 时请写成该目录的绝对路径。
            </p>
            <p className="row">
              <a className="btn btn-primary small" href="/downloads/smhook.exe" download="smhook.exe">
                下载 smhook.exe
              </a>
              <a className="btn btn-primary small" href="/downloads/shared-memory-mcp.exe" download="shared-memory-mcp.exe">
                下载 shared-memory-mcp.exe
              </a>
            </p>
          </li>
          <li>
            <strong>写入凭据文件</strong>
            <p className="small muted">钩子不读 MCP 配置。保存为当前用户目录下的 <code>%USERPROFILE%\.shared-memory\credentials.json</code>（或 <code>%SHARED_MEMORY_HOME%\credentials.json</code>）。</p>
            <CodeBlock label="credentials.json">
              <pre>{JSON.stringify(cred, null, 2)}</pre>
            </CodeBlock>
          </li>
          <li>
            <strong>合并 Cursor 钩子</strong>
            <p className="small muted">
              编辑 <code>%USERPROFILE%\.cursor\hooks.json</code>。只把下面三段加进已有的 <code>hooks</code>，不要覆盖已有的 sessionStart。
            </p>
            <CodeBlock label="追加到 hooks">
              <pre>{JSON.stringify(hooks, null, 2)}</pre>
            </CodeBlock>
            <p className="small muted">
              beforeSubmitPrompt 记你发出的内容，afterAgentResponse 记助手回复，subagentStop 记子智能体结果。
            </p>
          </li>
          <li>
            <strong>配置 MCP（只用于召回）</strong>
            <p className="small muted">
              编辑 <code>%USERPROFILE%\.cursor\mcp.json</code>，在 mcpServers 里加入或替换 shared-memory。保存后在 Cursor 里重载 MCP，直到出现 memory_recall。
            </p>
            <CodeBlock label="mcpServers.shared-memory">
              <pre>{JSON.stringify(mcp, null, 2)}</pre>
            </CodeBlock>
          </li>
          <li>
            <strong>加上召回规则</strong>
            <p className="small muted">新建 <code>%USERPROFILE%\.cursor\rules\shared-memory.mdc</code>。这条规则只提高召回概率。</p>
            <CodeBlock label="shared-memory.mdc">
              <pre>{rule}</pre>
            </CodeBlock>
          </li>
          <li>
            <strong>确认写成了</strong>
            <p className="small">
              完全退出 Cursor 再打开。发一句能认出来的话。回到本页，该连接的「最后活动」应从「—」变成时间；「历史」里应出现刚才的会话。
              没有新会话时，看 Cursor 的 Hooks 输出。召回是否发生看「回执」：没有回执只说明这次模型没调用 memory_recall。
            </p>
          </li>
        </ol>
      </div>
    </Modal>
  );
}
