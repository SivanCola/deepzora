import { useI18n } from "./i18n";

export type BotProvider = "weixin" | "feishu" | "lark" | "telegram";
export type BotConfig = {
  id: string;
  name: string;
  provider: BotProvider;
  appId: string;
  sessionId: string;
  chatId: string;
  senderId: string;
  mode?: "follow" | "dedicated" | "member";
  memberId?: string;
  legacySessionId?: string;
  modelRef?: string;
  workspaceId?: string;
};
export type BotConnection = BotConfig & {
  hasSecret: boolean;
  status: string;
  error: string;
  actualModel?: string;
  modelError?: string;
  enabled?: boolean;
  received?: number;
  sent?: number;
  lastReceivedAt?: string;
  lastSentAt?: string;
  memberName?: string;
};
export type BotModel = {
  ref: string;
  provider: string;
  providerLabel: string;
  model: string;
  configured: boolean;
};
export type BotWorkspace = { id: string; title: string; root: string };
export type BotSession = { id: string; title: string; workspace: string };
export type BotChannelsView = {
  connections: BotConnection[];
  sessions: BotSession[];
  models?: BotModel[];
  workspaces?: BotWorkspace[];
  members?: BotMember[];
};
export type BotMember = {
  id: string;
  name: string;
  model: string;
  permission: string;
  folder: string;
};
export type BotLoginView = {
  id: string;
  image: string;
  status: string;
  userId?: string;
  chatId?: string;
  appId?: string;
  provider?: BotProvider;
  interval?: number;
};
export interface BotChannelBindings {
  OpenBotMemberHistory(id: string): Promise<void>;
  TestBotModel(ref: string): Promise<void>;
  BeginFeishuBotLogin(provider: string): Promise<BotLoginView>;
  PollFeishuBotLogin(id: string): Promise<BotLoginView>;
  BeginWeixinBotLogin(): Promise<BotLoginView>;
  PollWeixinBotLogin(id: string): Promise<BotLoginView>;
  ListBotChannels(): Promise<BotChannelsView>;
  SaveBotConnection(
    input: BotConfig & { secret: string; loginId?: string; connect?: boolean },
  ): Promise<void>;
  DeleteBotConnection(id: string): Promise<void>;
  ConnectBotChannel(id: string): Promise<void>;
  DisconnectBotChannel(id: string): Promise<void>;
}
export const botProviders: BotProvider[] = [
  "weixin",
  "feishu",
  "lark",
  "telegram",
];
const en = {
  title: "Bot Channels",
  member: "Team member",
  memberHint:
    "This channel uses its member’s conversation, model, memory and permissions. Replies go only to the channel that started the task; Desktop and team messages are not broadcast.",
  noMembers:
    "Create and save a member in Team settings before adding a channel.",
  memberMissing: "Member unavailable — restore it in Team settings.",
  memberFixed:
    "The member binding is fixed once saved. Add another channel to use another member.",
  channelsSection: "Messaging channels",
  memberSavedFirst: "Save this member first, then add Weixin or Feishu here.",
  originalHistory: "Open original conversation",
  intro: "Continue your work from a messaging app.",
  mobile: "Connect from your phone",
  later:
    "Mobile Web access is coming in a later phase. Use a Bot Channel to work from your phone today.",
  channels: "Use a Bot Channel",
  channelsHint: "Connect a messaging bot to a local conversation.",
  configure: "Configure channel",
  scanConfigure: "Set up with QR code",
  feishuLogin: "Scan to create and configure bot",
  feishuPending:
    "Scan with Feishu or Lark and complete app creation and authorization on your phone.",
  feishuSuccess:
    "Bot authorized. App credentials and your private-chat access are ready. Choose a model mode and save, then connect.",
  feishuMissing:
    "App authorized, but no user identity was returned. Scan again or enter the allowed chat and sender manually.",
  denied:
    "Authorization declined. You can scan again or enter an existing app manually.",
  feishuAccessHint:
    "QR setup binds the scanning account’s private conversation automatically. Manual setup accepts a chat_id and sender open_id.",
  manage: "Manage bots",
  empty:
    "No bots configured. Choose a channel above to add your first connection.",
  name: "Connection name",
  modelMode: "Model mode",
  follow: "Follow shared conversation",
  dedicated: "Dedicated conversation and model",
  dedicatedHint:
    "Creates a private conversation for this bot on first connection. Reconnecting keeps its history. Changing model or workspace starts a separate conversation; earlier history remains in Desktop.",
  modelService: "Model service",
  model: "Model",
  workspace: "Workspace",
  globalWorkspace: "Default workspace",
  modelHint:
    "Uses the credentials saved in Model Services. A configured key has not necessarily passed authentication.",
  noModels: "Add a model connection and API key in Model Services first.",
  missingKey: "API key required",
  modelUnavailable: "Saved model unavailable — choose another model",
  testModel: "Test model",
  testingModel: "Testing model…",
  modelPassed: "Model test passed. This does not connect the bot.",
  configuredModel: "Configured model",
  actualModel: "Current model",
  session: "Shared conversation",
  sessionHint:
    "While connected, assistant replies in this conversation are sent to the allowed chat, including tasks started on Desktop. Keep this conversation open.",
  noSession: "Open a local conversation in Desktop first.",
  chat: "Allowed chat ID",
  sender: "Allowed sender ID",
  accessHint:
    "Both must match. Other users and chats cannot control this conversation.",
  appId: "App ID",
  secret: "Bot token / app secret",
  secretHint:
    "Stored in a local file accessible only to your user account. Never returned to the interface. Leave blank to keep the saved credential.",
  save: "Save connection",
  saveOnly: "Save only",
  saveConnect: "Save and connect",
  saveReconnect: "Save and reconnect",
  retryConnect: "Retry connection",
  delivery: "Messages since connection",
  received: "Received",
  sent: "Send confirmed",
  enabledHint: "Enabled · reconnects after app restart",
  saved: "Configuration saved. Connection status is shown below.",
  cancel: "Cancel",
  edit: "Edit",
  remove: "Delete",
  confirm: "Delete connection?",
  connect: "Connect",
  disconnect: "Disconnect",
  refresh: "Refresh",
  back: "Back to bots",
  new: "New connection",
  dirty: "Unsaved changes",
  clean: "No changes",
  loading: "Loading connections…",
  retry: "Retry",
  stopped: "Stopped",
  connecting: "Connecting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  error: "Connection error",
  unknown: "Unknown status",
  china: "China",
  global: "Global",
  weixin: "Weixin",
  feishu: "Feishu",
  lark: "Lark",
  telegram: "Telegram",
  telegramHelp:
    "Create a bot with BotFather. Enter its token, the chat ID and your numeric user ID. The bot must not be running in another app or have an active webhook.",
  feishuHelp:
    "Scan to create and authorize a personal bot. App ID, secret and private-chat access are filled automatically. You can also enter an existing app manually; enable bot messaging and im.message.receive_v1 long-connection events in its console.",
  weixinHelp:
    "Scan with Weixin to sign in. Both access IDs are filled automatically from your Weixin identity. After connecting, send the bot a message to establish reply context.",
  commands:
    "Text messages start tasks. /help shows commands; /stop cancels; /approve ID and /deny ID answer approvals. Attachments and structured questions are handled in Desktop.",
  restart:
    "Enabled bots reconnect after app restart and after saving edits. Disconnect keeps a bot stopped across restarts. Send a new message once connected; messages from before this connection are skipped.",
  login: "Sign in with Weixin QR",
  pending: "Scan with Weixin and confirm on your phone.",
  scanned: "Scanned. Confirm on your phone.",
  expired: "QR code expired. Start sign-in again.",
  authorized: "Signed in. Save this connection within 10 minutes.",
  accessIdentified:
    "Signed in. Both access IDs have been filled for the Weixin account that scanned the QR code. Choose a model mode and save.",
  accessMissing:
    "Signed in, but Weixin did not return a user ID. Scan again or enter the private-chat iLink ID manually; the bot ID is not the sender ID.",
  weixinAccessHint:
    "QR login fills both fields with the scanning account’s iLink user ID. This is not your visible Weixin username. You can edit it to allow another account.",
};
const zh: Record<keyof typeof en, string> = {
  title: "机器人渠道",
  member: "团队成员",
  memberHint:
    "渠道统一使用成员的会话、模型、记忆和权限。只回复发起任务的渠道；桌面消息及团队内部交办不会广播到微信或飞书。",
  noMembers: "请先在团队设置中创建并保存成员，再为其添加渠道。",
  memberMissing: "成员不可用，请在团队设置中恢复成员。",
  memberFixed: "保存后固定绑定此成员。需要其他成员时，请为其新增渠道。",
  channelsSection: "消息渠道",
  memberSavedFirst: "请先保存成员，再在此处添加微信或飞书。",
  originalHistory: "打开原会话历史",
  intro: "从聊天应用继续处理你的工作。",
  mobile: "手机连接",
  later: "手机 Web 访问将在后续阶段提供。目前可通过右侧 Bot 渠道在手机上使用。",
  channels: "使用 Bot Channel",
  channelsHint: "将聊天机器人连接到本地会话。",
  configure: "配置渠道",
  scanConfigure: "扫码配置",
  feishuLogin: "扫码创建并配置机器人",
  feishuPending: "使用飞书或 Lark 扫码，在手机上完成应用创建与授权。",
  feishuSuccess:
    "机器人已授权，应用凭据和你的私聊访问对象已自动配置。选择模型模式并保存后，即可连接。",
  feishuMissing:
    "应用已授权，但未返回用户身份。请重新扫码，或手动填写允许的聊天和发送者。",
  denied: "已拒绝授权。可以重新扫码，或手动填写已有应用。",
  feishuAccessHint:
    "扫码自动绑定扫码账号的私聊。手动配置时可填写 chat_id 和发送者 open_id。",
  manage: "机器人管理",
  empty: "尚未配置机器人。选择上方渠道，添加第一个连接。",
  name: "连接名称",
  modelMode: "模型模式",
  follow: "跟随绑定会话",
  dedicated: "指定模型与专属会话",
  dedicatedHint:
    "首次连接时为此 Bot 创建专属会话，重连继续保留上下文。更换模型或工作区会使用另一专属会话，旧记录仍保留在桌面侧栏。",
  modelService: "模型服务",
  model: "模型",
  workspace: "工作区",
  globalWorkspace: "默认工作区",
  modelHint:
    "复用“模型服务”中保存的凭据。已配置密钥不代表认证成功，可先测试模型。",
  noModels: "请先在“模型服务”中添加模型连接并配置 API Key。",
  missingKey: "需要配置 API Key",
  modelUnavailable: "已保存的模型不可用，请重新选择",
  testModel: "测试模型",
  testingModel: "正在测试模型…",
  modelPassed: "模型测试通过；机器人尚未因此连接。",
  configuredModel: "指定模型",
  actualModel: "当前模型",
  session: "共享会话",
  sessionHint:
    "连接期间，此会话中的助手回复会发送到允许的聊天，包括在桌面发起的任务。请保持此会话打开。",
  noSession: "请先在桌面打开一个本地会话。",
  chat: "允许的聊天 ID",
  sender: "允许的发送者 ID",
  accessHint: "两个 ID 必须同时匹配。其他用户和聊天无法控制此会话。",
  appId: "App ID",
  secret: "Bot Token / App Secret",
  secretHint:
    "保存在仅当前系统用户可读的本地文件中，不会回传到界面。留空保留已保存凭据。",
  save: "保存连接",
  saveOnly: "仅保存",
  saveConnect: "保存并连接",
  saveReconnect: "保存并重连",
  retryConnect: "重试连接",
  delivery: "本次连接消息",
  received: "已接收",
  sent: "发送已确认",
  enabledHint: "已启用 · 应用重启后自动连接",
  saved: "配置已保存，连接状态见下方列表。",
  cancel: "取消",
  edit: "编辑",
  remove: "删除",
  confirm: "确认删除连接？",
  connect: "连接",
  disconnect: "断开",
  refresh: "刷新",
  back: "返回机器人列表",
  new: "新增连接",
  dirty: "未保存更改",
  clean: "无更改",
  loading: "正在加载连接…",
  retry: "重试",
  stopped: "已停止",
  connecting: "连接中",
  connected: "已连接",
  reconnecting: "重新连接中",
  error: "连接错误",
  unknown: "未知状态",
  china: "中国",
  global: "全球",
  weixin: "微信",
  feishu: "飞书",
  lark: "Lark",
  telegram: "Telegram",
  telegramHelp:
    "通过 BotFather 创建机器人，填写 Token、聊天 ID 和你的数字用户 ID。此机器人不能同时被其他应用轮询，也不能有正在使用的 Webhook。",
  feishuHelp:
    "扫码创建并授权个人机器人，自动填入 App ID、凭据和私聊访问对象。也可手动填写已有应用；请在应用后台启用机器人消息权限及 im.message.receive_v1 长连接事件。",
  weixinHelp:
    "使用微信扫码登录，两个访问 ID 会根据扫码微信身份自动填入。连接后先给机器人发送一条消息，以建立回复上下文。",
  commands:
    "文本消息启动任务。/help 查看命令，/stop 取消任务，/approve ID 和 /deny ID 回答审批。附件和结构化提问在桌面处理。",
  restart:
    "已启用的机器人会在重启应用、保存修改后自动连接；手动断开后保持停止。请在已连接后发送新消息，开始连接前的旧消息会跳过。",
  login: "微信扫码登录",
  pending: "使用微信扫码，并在手机上确认。",
  scanned: "已扫码，请在手机上确认。",
  expired: "二维码已过期，请重新登录。",
  authorized: "登录成功，请在 10 分钟内保存连接。",
  accessIdentified:
    "登录成功，已自动填入扫码微信对应的两个访问 ID。选择模型模式后即可保存。",
  accessMissing:
    "登录成功，但微信未返回用户 ID。请重新扫码，或手动填写私聊对象的 iLink ID；机器人 ID 不能作为发送者 ID。",
  weixinAccessHint:
    "扫码后自动填入扫码账号的 iLink 用户标识，不是微信号。需要允许其他账号时，可手动修改。",
};
export function useBotText() {
  return useI18n().locale === "zh" ? zh : en;
}

// Browser preview never claims a real provider connection.
export function makeMockBotChannels(memberSource?:()=>Promise<BotMember[]>): BotChannelBindings {
  let connections: BotConnection[] = [];
  return {
    async OpenBotMemberHistory() {},
    async TestBotModel() {
      throw new Error("Model testing requires the Desktop app.");
    },
    async BeginFeishuBotLogin() {
      throw new Error("Bot QR setup requires the Desktop app.");
    },
    async PollFeishuBotLogin(id) {
      return { id, image: "", status: "expired" };
    },
    async BeginWeixinBotLogin() {
      throw new Error("Weixin sign-in requires the Desktop app.");
    },
    async PollWeixinBotLogin(id) {
      return { id, image: "", status: "expired" };
    },
    async ListBotChannels() {
      return {
        connections: structuredClone(connections),
        members: [...(memberSource?await memberSource():[]),
          {
            id: "a-0000000000000001",
            name: "Preview member",
            model: "preview/flash",
            permission: "read-only",
            folder: "/workspace",
          },
        ],
        sessions: [
          {
            id: "preview-session",
            title: "Preview conversation",
            workspace: "/workspace",
          },
        ],
        models: [
          {
            ref: "preview/flash",
            provider: "preview",
            providerLabel: "Preview service",
            model: "flash",
            configured: true,
          },
          {
            ref: "preview/pro",
            provider: "preview",
            providerLabel: "Preview service",
            model: "pro",
            configured: true,
          },
          {
            ref: "missing/model",
            provider: "missing",
            providerLabel: "Missing key",
            model: "model",
            configured: false,
          },
        ],
        workspaces: [
          { id: "global", title: "Global", root: "" },
          {
            id: "preview-workspace",
            title: "Preview project",
            root: "/workspace",
          },
        ],
      };
    },
    async SaveBotConnection({ secret, connect, ...input }) {
      const previous = connections.find((item) => item.id === input.id);
      connections = [
        ...connections.filter((item) => item.id !== input.id),
        {
          ...input,
          sessionId:
            input.mode === "dedicated"
              ? `desktop-bot-${input.id}`
              : input.sessionId,
          hasSecret: !!secret || !!previous?.hasSecret,
          enabled: !!connect || !!previous?.enabled,
          status: "stopped",
          error: "",
        },
      ];
    },
    async DeleteBotConnection(id) {
      connections = connections.filter((item) => item.id !== id);
    },
    async ConnectBotChannel() {
      throw new Error("Bot connections require the Desktop app.");
    },
    async DisconnectBotChannel(id) {
      connections = connections.map((item) =>
        item.id === id
          ? { ...item, enabled: false, status: "stopped", error: "" }
          : item,
      );
    },
  };
}
