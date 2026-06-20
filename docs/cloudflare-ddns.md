# Cloudflare DDNS 管理功能

这个分支把 DDNS 放在 Komari server 后端执行，不放进公开主题页面。Cloudflare API Token 只保存在服务端，管理员接口只返回 `token_configured`，不会把 Token 返回给浏览器。

## 已实现能力

- 管理员保存 / 移除 Cloudflare DDNS Token。
- 管理员为指定 Komari 服务器绑定 DNS 记录。
- 从 Komari agent 上报的 `ipv4` / `ipv6` 字段读取当前 IP。
- IP 没变化时不调用 Cloudflare，避免无意义请求。
- 支持手动同步和后台定时同步。
- 同步结果写入 `current_ip`、`last_ip`、`last_status`、`last_error`、`last_sync_at`。

## Cloudflare Token 权限

创建 Cloudflare API Token 时只给最小权限：

- `Zone:DNS:Edit`
- `Zone:Zone:Read`
- 资源范围只选择需要 DDNS 的 Zone，不要选择所有域名。

也可以用环境变量配置 Token：

```bash
KOMARI_CLOUDFLARE_DDNS_TOKEN="你的 Cloudflare API Token"
```

环境变量优先级高于数据库里保存的 Token。

## 管理员接口

所有接口都在 `/api/admin/ddns` 下，需要管理员登录或管理员 API Key。

查看 Token 状态：

```http
GET /api/admin/ddns/provider?provider=cloudflare
```

保存 Token：

```http
POST /api/admin/ddns/provider
Content-Type: application/json

{
  "provider": "cloudflare",
  "token": "Cloudflare API Token"
}
```

移除 Token：

```http
POST /api/admin/ddns/provider/remove-token
Content-Type: application/json

{
  "provider": "cloudflare"
}
```

列出 DDNS 记录：

```http
GET /api/admin/ddns/
```

新增或更新 DDNS 记录：

```http
POST /api/admin/ddns/record
Content-Type: application/json

{
  "enabled": true,
  "provider": "cloudflare",
  "client": "Komari 节点 UUID",
  "zone_id": "Cloudflare Zone ID",
  "record_id": "Cloudflare DNS Record ID",
  "record_name": "jp-lite.example.com",
  "record_type": "A",
  "ip_source": "client_ipv4",
  "ttl": 1,
  "proxied": false
}
```

`record_type` 支持：

- `A`：必须使用 `client_ipv4`
- `AAAA`：必须使用 `client_ipv6`

`ttl` 支持：

- `1`：Cloudflare 自动 TTL
- `60` 到 `86400`：固定秒数

手动同步全部记录：

```http
POST /api/admin/ddns/sync
Content-Type: application/json

{}
```

手动同步单条记录：

```http
POST /api/admin/ddns/sync
Content-Type: application/json

{
  "id": 1,
  "force": true
}
```

删除记录：

```http
POST /api/admin/ddns/record/delete
Content-Type: application/json

{
  "ids": [1, 2]
}
```

启用 / 禁用记录：

```http
POST /api/admin/ddns/record/enable
POST /api/admin/ddns/record/disable
Content-Type: application/json

{
  "ids": [1, 2]
}
```

## 定时同步

服务启动后会注册定时任务：

```text
ddns:sync @every 2m
```

任务会立即执行一次，然后每 2 分钟同步一次已启用的记录。同步时如果 IP 没变化，不会请求 Cloudflare 更新 DNS。

## 安全边界

- 公开主题页面不需要、也不应该调用这些接口。
- Token 字段使用 AES-GCM 加密保存，密钥来自 `KOMARI_SECRET_KEY` 或 `./data/secret.key`。
- API 返回不会包含真实 Token。
- 同步错误会过滤 Token 字符串，避免误写入错误信息。
- 公开节点接口仍按原有逻辑隐藏或打码 IP，不因为 DDNS 功能改变。

## Cloudflare 配置步骤

1. 在 Cloudflare DNS 页面先创建一条 `A` 或 `AAAA` 记录。
2. 进入该记录的详情或通过 Cloudflare API 获取 `record_id`。
3. 在域名首页右侧复制 `zone_id`。
4. 创建最小权限 API Token。
5. 在 Komari 管理员接口保存 Token。
6. 新增 DDNS 记录，选择对应 Komari 节点 UUID。
7. 点手动同步，确认 `last_status` 为 `success`。

参考：

- [Cloudflare API Token](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)
- [Cloudflare DNS Records API](https://developers.cloudflare.com/api/resources/dns/subresources/records/)
