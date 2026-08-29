# WxPusher 脱敏响应夹具

这些文件只保存供应商响应的字段结构，不包含真实 `appToken`、UID、消息正文或用户信息。

- `status-pending-numeric.sanitized.json`：根据 2026-08-26 真实联调中已经确认的“数字型 `data` + 顶层 `msg`”结构重建；原始报文未留存，因此本文件是脱敏结构样本，不是原始网络抓包。
- `send-accepted.sanitized.json`：符合官方发送接口和现有隔离测试的受理响应合同；接收者与记录 ID 均为测试占位值。
- `status-succeeded-object.sanitized.json`：对象型最终成功状态的合同样本。

下一次真实联调如观察到新的响应形态，应先移除所有密钥、UID、消息内容和可追踪记录 ID，再新增独立夹具及对应解析测试。任何原始生产响应都不得提交到仓库。
