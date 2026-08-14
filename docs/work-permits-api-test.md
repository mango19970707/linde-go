# `/api/work-permits` 综合测试文档

## 1. 测试目标

验证核心作业票流程是否可用：

- 首次请求能正常受理
- 信息不足时能返回补充问题
- 票种待确认时能返回候选票种
- 审批前能返回待提交内容
- 续传 `session_id` 后能继续流程直到完成

## 2. 启动服务

默认端口为 `7070`。

```bash
go run .
```

如需自定义配置：

```bash
LINDE_CONFIG=configs/config.yaml go run .
```

基础地址：

```bash
export BASE_URL=http://127.0.0.1:7070
```

Windows PowerShell:

```powershell
$env:BASE_URL="http://127.0.0.1:7070"
```

## 3. 首次请求测试

### 3.1 发起一次作业票请求

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "input": "在空分东侧进行焊接作业，时间今天 08:00 到 10:00",
    "site_id": 28,
    "user_id": 464
  }'
```

### 3.2 预期结果

返回 `200`，且 `status` 通常为以下之一：

- `need_more_info`：信息不足，需补充
- `need_wp_type_confirmation`：需要确认票种
- `need_approval`：已生成待提交内容，等待审批
- `completed`：流程直接完成

所有非报错场景都应返回 `session_id`。

## 4. 补充信息测试

适用于上一步返回 `status=need_more_info`。

### 4.1 记录首次返回的 `session_id`

示例返回：

```json
{
  "session_id": "replace-session-id",
  "status": "need_more_info",
  "missing_fields": ["start_time", "end_time"],
  "question": "please provide start and end time"
}
```

### 4.2 使用同一个 `session_id` 补充信息

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "replace-session-id",
    "input": "开始时间 08:00，结束时间 10:00",
    "site_id": 28,
    "user_id": 464
  }'
```

### 4.3 关键检查

- 返回 `200`
- `session_id` 不变
- `status` 进入下一阶段，通常为 `need_wp_type_confirmation`、`need_approval` 或 `completed`

## 5. 票种确认测试

适用于返回 `status=need_wp_type_confirmation`。

### 5.1 示例返回

```json
{
  "session_id": "replace-session-id",
  "status": "need_wp_type_confirmation",
  "question": "confirm permit types",
  "intent_wp_types": [
    {
      "id": 243,
      "templetname": "permit",
      "wptypename": "PTW800",
      "wptypeid": 23
    }
  ]
}
```

### 5.2 确认票种

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "replace-session-id",
    "confirmed": true,
    "intent_wp_types": [
      {
        "id": 243,
        "templetname": "permit",
        "wptypename": "PTW800",
        "wptypeid": 23
      }
    ],
    "site_id": 28,
    "user_id": 464
  }'
```

### 5.3 关键检查

- 返回 `200`
- `status` 进入 `need_approval` 或 `completed`

## 6. 审批提交测试

适用于返回 `status=need_approval`。

### 6.1 示例返回

```json
{
  "session_id": "replace-session-id",
  "status": "need_approval",
  "filled_wps": [
    {
      "wp_type": "一级动火",
      "name": "permit"
    }
  ]
}
```

### 6.2 同意提交

`payloads` 建议直接使用上一步接口返回或前端展示后确认过的完整内容；下面只给最小示例。

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "replace-session-id",
    "approved": true,
    "payloads": [
      {
        "id": 0,
        "wptypeid": 23
      }
    ],
    "site_id": 28,
    "user_id": 464
  }'
```

### 6.3 拒绝提交

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "replace-session-id",
    "approved": false,
    "site_id": 28,
    "user_id": 464
  }'
```

### 6.4 关键检查

- 返回 `200`
- 成功时 `status=completed`
- 若后端有业务提示，检查 `message` 字段

## 7. 异常用例

### 7.1 缺少 `site_id` 或 `user_id`

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "input": "test"
  }'
```

预期：

```json
{
  "error": "site_id and user_id are required"
}
```

### 7.2 缺少首次 `input`

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "site_id": 28,
    "user_id": 464
  }'
```

预期包含：

```json
{
  "error": "input is required"
}
```

### 7.3 使用不存在的 `session_id`

```bash
curl -X POST "$BASE_URL/api/work-permits" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "not-exists",
    "input": "test",
    "site_id": 28,
    "user_id": 464
  }'
```

预期包含：

```json
{
  "error": "session not found"
}
```

## 8. 验收标准

满足以下条件即可认为核心流程通过：

- 首次请求能返回有效 `session_id`
- 中断场景下可基于同一 `session_id` 成功续传
- 至少完成一次 `need_more_info` -> `completed` 或 `need_approval` -> `completed`
- 参数缺失时能返回明确错误信息
