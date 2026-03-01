# Agent 节点接入指南 🤖

这个目录包含了将你的 Agent 接入 AgentPay 网络的示例代码和配置说明。

## 接入步骤

### 1. 实现核心接口
你的 Agent 需要暴露两个核心接口给 AgentPay Router：

- **`GET /health`**: 健康检查。Router 定期调用，确保你的节点在线。
- **`POST /chat`**: 业务执行。通过校验 `X-402-Proof` Header 来执行支付验证。

### 2. 自动注册
Agent 启动后会通过 `autoRegister` 函数向 Router 注册。你需要配置正确的 `ROUTER_URL` 以及你的收款地址 `AGENT_RECIPIENT`。

## 快速开始

1. **环境准备**:
   复制并重命名 `.env.example` 到 `.env`：
   ```env
   PORT=9000
   ROUTER_URL=http://api.agentpay.space:8080
   AGENT_NAME=MyExpertAgent
   AGENT_RECIPIENT=0x你的收款地址
   AGENT_PRICE=1000000  # 1 USDC
   ```

2. **启动 Agent**:
   ```bash
   go run ./cmd/agent/main.go
   ```

## 核心配置详解
- **`AGENT_PRICE`**: 调用一次你的 Agent 所需消耗的 USDC（单位为 micro-USDC，即 1,000,000 = 1 USDC）。
- **`AGENT_ENDPOINT`**: 你的 Agent 所在的外网可访问地址。
- **`ROUTER_URL`**: AgentPay Router 的注册中心地址。
