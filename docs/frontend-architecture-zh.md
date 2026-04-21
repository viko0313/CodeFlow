# CodeFlow 前端技术方案文档

本文档详细描述了 CodeFlow Agent 本地 Web 工作区的架构设计与技术实现方案。

## 1. 核心技术栈 (Tech Stack)

CodeFlow 前端采用现代化的 React 生态体系，重点关注实时交互、高性能编辑器集成以及轻量化状态管理。

- **框架**: [Next.js 15+](https://nextjs.org/) (App Router) - 利用 RSC (React Server Components) 优化首屏加载，提供高效的 API 路由。
- **状态管理**: [Zustand](https://github.com/pmndrs/zustand) - 极简的全局状态管理，用于处理编辑器状态、Socket 状态及权限审批。
- **实时通信**: 原生 **WebSocket** - 用于双向流式对话、终端输出实时同步及工具调用审批。
- **数据获取**: [TanStack Query v5](https://tanstack.com/query) - 负责 RESTful API 的缓存、同步与错误处理。
- **认证**: [NextAuth.js v5 (Auth.js)](https://authjs.dev/) - 提供轻量的本地身份校验。
- **UI 组件库**: [Radix UI](https://www.radix-ui.com/) + [Tailwind CSS 4.0](https://tailwindcss.com/) - 响应式、可访问性友好的组件体系。
- **核心组件**:
  - **编辑器**: [@monaco-editor/react](https://github.com/suren-atoyan/monaco-react) - 提供 VS Code 级别的代码查看与编辑体验。
  - **终端**: [@xterm/xterm](https://xtermjs.org/) - 模拟真实的终端输出。
  - **图表**: [Recharts](https://recharts.org/) - 用于 Dashboard 审计日志的统计展示。

## 2. 架构设计 (Architecture)

### 2.1 目录结构
```text
web/
├── src/
│   ├── app/                # Next.js App Router (页面与路由)
│   │   ├── dashboard/      # 审计与统计面板
│   │   ├── ide/            # 核心 IDE 交互界面
│   │   └── login/          # 认证页面
│   ├── components/         # 可复用 UI 组件
│   │   ├── ide/            # IDE 专用组件 (Terminal, Monaco, Approval)
│   │   └── ui/             # 基础原子组件 (Button, Input, etc.)
│   ├── hooks/              # 自定义 Hooks (Socket 逻辑、状态订阅)
│   ├── lib/                # 工具函数、API 定义、事件 Reducer
│   └── stores/             # Zustand 状态定义
```

### 2.2 核心交互流
1. **REST API**: 用于获取静态配置（Skills, MCP）、历史 Session 列表、健康检查及文档上传。
2. **WebSocket**: 核心对话逻辑。
   - 客户端通过 `useCodeFlowSocket` 钩子维护连接。
   - 后端推送 `chat.token` 实现流式渲染。
   - `reduceServerEvent` (Reducer) 统一处理服务器推送的各种事件（终端输出、Diff 预览、权限请求等）。

## 3. 核心功能实现方案

### 3.1 权限审批流 (Permission/Approval)
当前端接收到 `permission.required` 事件时：
- UI 状态机（Zustand）捕获 `pendingApproval` 状态。
- 弹出 `ApprovalDialog`，展示操作风险、超时时间、命令详情或文件 Diff。
- 用户操作（同意/拒绝）通过 WebSocket 发回 `chat.approve` 消息。

### 3.2 IDE 仿真体验
- **Monaco Editor**: 实时展示后端 Agent 正在修改的文件内容，支持 Diff 模式。
- **Xterm.js**: 捕获后端 Shell 工具的实时输出，提供接近原生的终端体验。
- **Event Timeline**: 记录 Agent 的每一个决策步骤（思考、调用工具、结果反馈）。

### 3.3 数据持久化与缓存
- 使用 TanStack Query 对 Session 列表和配置信息进行 SWR (Stale-While-Revalidate) 处理。
- 本地 Session 状态通过 URL 参数 (`session_id`) 进行同步，确保页面刷新后仍能恢复连接。

## 4. 开发与部署
- **环境变量**: 依赖 `AUTH_SECRET` 进行加密。
- **代理配置**: Next.js 配置将 `/api/codeflow/*` 代理至 Go 后端服务 (`localhost:8742`)。
- **构建**: 支持标准 `next build`，可生成高性能的生产环境静态资源。
