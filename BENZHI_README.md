# BENZHI_README

基于 Go 实现的manuscript-conservation-gate Web 项目，一款后端服务，已完整实现面向古籍修复室的修复方案治理工作台，覆盖建档、逐页损伤基线、不可覆盖的版本化方案、材料相容性规则、模拟试样、专家退回整改与再提交、方案冻结及不可变放行凭据；本地事件日志和审计记录均采用链式哈希，并支持投影原子更新与启动恢复。

## 项目说明
- 项目：benzhi-project-8c584b5a-ddec-4b06-80b3-556db55d23dc
- 项目用途：已完整实现面向古籍修复室的修复方案治理工作台，覆盖建档、逐页损伤基线、不可覆盖的版本化方案、材料相容性规则、模拟试样、专家退回整改与再提交、方案冻结及不可变放行凭据；本地事件日志和审计记录均采用链式哈希，并支持投影原子更新与启动恢复。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/conservation -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-8c584b5a-ddec-4b06-80b3-556db55d23dc-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-8c584b5a-ddec-4b06-80b3-556db55d23dc-arm64 linux/arm64
docker run -it benzhi-project-8c584b5a-ddec-4b06-80b3-556db55d23dc-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/conservation -selfcheck -addr=127.0.0.1:19081`
