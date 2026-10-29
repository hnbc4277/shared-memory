import { CodeBlock, Modal } from "../ui";
import { usePublicApiUrl } from "../usePublicApiUrl";

function StepBadge({ kind }: { kind: "auto" | "manual" | "check" }) {
  if (kind === "manual") {
    return <span className="setup-step-badge setup-step-badge-manual">必须手动</span>;
  }
  if (kind === "auto") {
    return <span className="setup-step-badge setup-step-badge-auto">脚本可完成</span>;
  }
  return <span className="setup-step-badge setup-step-badge-check">建议自检</span>;
}

/** One Doubao form field: field name as CodeBlock label, value to copy. */
function FieldCopy({
  field,
  hint,
  value,
}: {
  field: string;
  hint?: string;
  value: string;
}) {
  return (
    <div className="doubao-field-copy">
      {hint ? <p className="small muted doubao-field-hint">{hint}</p> : null}
      <CodeBlock label={field} copyText={value}>
        <pre>{value}</pre>
      </CodeBlock>
    </div>
  );
}

export function DoubaoSetupGuide({
  connectionId,
  apiKey,
  onClose,
}: {
  connectionId: string;
  apiKey?: string;
  onClose?: () => void;
}) {
  const hasKey = Boolean(apiKey && apiKey.length > 0);
  const key = hasKey ? apiKey! : "<把创建连接时显示的专用密钥贴这里>";
  const publicApi = usePublicApiUrl();
  // Doubao STDIO 表单通常不展开 %USERPROFILE%，请用绝对路径。
  const mcpCommand = "C:\\Users\\<你的Windows用户名>\\.shared-memory\\shared-memory-mcp.exe";

  return (
    <Modal title="豆包配置步骤" onClose={onClose ?? (() => {})} wide>
      <div id="doubao-setup-guide">
        <p className="small muted">
          豆包没有 Cursor 那种静默 hook：<strong>普通聊天不会自动出现在平台</strong>。
          「读取在线」只表示 MCP 心跳；会话历史要等模型调用 <code>memory_ingest</code> 之后才有。
          请在<strong>豆包工作 / Agent 模式</strong>里用连接器 + 本 Skill（不要只在普通闲聊窗口里期待同步）。
        </p>
        <ol className="cursor-setup-steps">
          <li>
            <strong>下载并运行配置脚本</strong> <StepBadge kind="auto" />
            <p className="small muted">
              创建时勾选「专用 API 密钥」后，行内「下载配置脚本」解压双击 <code>.cmd</code>。
              脚本会写入 <code>%USERPROFILE%\.shared-memory\doubao\</code>、凭据，并尽量自动把 Skill 拷进豆包
              <code>.user_skills\shared-memory\</code>。请用<strong>与豆包同一 Windows 用户</strong>执行。
            </p>
          </li>
          <li>
            <strong>核对本机材料</strong> <StepBadge kind="check" />
            <CodeBlock label="connection_id">
              <pre>{connectionId}</pre>
            </CodeBlock>
            {hasKey ? (
              <CodeBlock label="专用 API 密钥">
                <pre>{apiKey}</pre>
              </CodeBlock>
            ) : (
              <p className="small muted">
                未取到密钥时，到本机 <code>%USERPROFILE%\.shared-memory\credentials.json</code> 或安装目录{" "}
                <code>mcp.connector.json</code> 复制；没有则吊销后重建并勾选「专用 API 密钥」。
              </p>
            )}
          </li>
          <li>
            <strong>在豆包工作里添加自定义 MCP 连接器</strong> <StepBadge kind="manual" />
            <p className="small muted">
              打开：左侧「插件·技能·伙伴」→ 插件 / 连接器 →「+」→「新建自定义连接器」。
              下面每一项对应豆包表单里的一个框：标题是字段名，下方灰框是要复制的内容。
            </p>

            <FieldCopy field="传输类型" hint="在下拉里选择 STDIO（一般不用复制）" value="STDIO" />
            <FieldCopy field="名称" value="shared-memory" />
            <FieldCopy
              field="命令"
              hint="只把路径贴到「命令」；把 <你的Windows用户名> 改成本机用户名（如 30493）"
              value={mcpCommand}
            />
            <p className="small muted doubao-field-note">「参数」留空。</p>

            <p className="small muted doubao-env-heading">环境变量（点「添加」，每条一对名/值）</p>
            <FieldCopy field="变量名 1" value="SHARED_MEMORY_API_URL" />
            <FieldCopy
              field="变量值 1"
              hint="来自 API 配置 PUBLIC_API_URL；本机同机可改成 http://127.0.0.1:18080"
              value={publicApi}
            />
            <FieldCopy field="变量名 2" value="SHARED_MEMORY_API_KEY" />
            <FieldCopy field="变量值 2" value={key} />
            <FieldCopy field="变量名 3" value="SHARED_MEMORY_AGENT" />
            <FieldCopy field="变量值 3" value="doubao" />
            <FieldCopy field="变量名 4" value="SHARED_MEMORY_CONNECTION_ID" />
            <FieldCopy field="变量值 4" value={connectionId} />

            <p className="small muted">
              保存后若运行失败，到「我安装的」点「重新启动」。连接器在跑时本页「心跳」应更新。
            </p>
          </li>
          <li>
            <strong>确认 Skill 已出现</strong> <StepBadge kind="check" />
            <p className="small muted">
              「技能」→「我安装的」里应有 <code>shared-memory</code>。没有则重启豆包，或把安装包里的{" "}
              <code>SKILL.md</code> 拷到本机{" "}
              <code>%LOCALAPPDATA%\Doubao\User Data\Default\.doubao\agent_mode\workspace\.user_skills\shared-memory\</code>。
            </p>
          </li>
          <li>
            <strong>验证写入（否则平台永远没有聊天记录）</strong> <StepBadge kind="check" />
            <p className="small muted">
              在<strong>新的工作任务</strong>里用 <code>/</code> 点选 <code>shared-memory</code>，或直接说：
              「用 Shared Memory：先 recall，再把这句话 ingest 进去：测试同步」。
            </p>
            <p className="small muted">
              成功后本页「活跃」不再是「尚无活动」，「查看会话」才能看到记录。仅心跳在线、没有 ingest，平台不会有聊天。
            </p>
          </li>
        </ol>
      </div>
    </Modal>
  );
}
