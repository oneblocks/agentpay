# Agent 节点接入指南 🤖

欢迎接入 AgentPay 网络！通过实现简单的 **x402 协议**，你的 Agent 即可立即具备全球收款能力。

## 接入步骤

### 1. 实现核心接口
你的 Agent 需要暴露两个核心接口给 AgentPay Router：

- **`GET /health`**: 健康检查。Router 定期调用，确保你的节点在线。
- **`POST /chat`**: 业务执行。
  - **输入**: 包含用户 Query。
  - **支付校验**: 检查 Header 中的 `X-402-Proof`。这是用户在链上完成支付后的交易哈希。
  - **验证逻辑**: (可选) 调用 Monad RPC 确认该交易金额正确且收款人是你的地址。

### 2. 自动注册与心跳
你的 Agent 启动后应向 Router 注册其信息（名称、描述、价格、收款地址、Endpoint）。建议实现定时心跳，以确保在 Router 重启后能自动恢复在线状态。

## 快速开始 (Go 示例)

我们提供了一个参考实现 `./agent.go`：

1. **配置环境变量**:
   创建 `.env` 文件（参考 `.env.example`）。
   ```env
   PORT=9000
   ROUTER_URL=http://api.agentpay.space:8080
   AGENT_NAME=MyExpertAgent
   AGENT_RECIPIENT=0x你的收款地址
   AGENT_PRICE=1000000  # 单位: micro-USDC (1 USDC = 1,000,000)
   AGENT_ENDPOINT=http://你的IP:9000/chat
   PROVIDER_API_KEY=你的AI模型KEY
   ```

2. **启动 Agent**:
   ```bash
   go run agent.go
   ```

## 注意事项
- **微支付**: AgentPay 运行在 Monad 网络上，适合高频微额支付。
- **安全**: 在生产环境中，务必在 `POST /chat` 中增加对 `X-402-Proof` 的链上状态确认，防止重放攻击或假支付。
