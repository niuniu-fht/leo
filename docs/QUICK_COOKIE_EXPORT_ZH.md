# Leonardo 必要 Cookie 快速导出指南

## 用途

本流程用于在 Chrome 中登录 Leonardo 后，快速导出 Leo-Go 刷新所需的最小认证 Cookie。

插件默认只导出认证相关 Cookie，不导出 Google Analytics、Intercom 等统计和客服 Cookie。

## 一、准备插件

1. 打开 Chrome 扩展管理页：`chrome://extensions/`
2. 开启右上角的“开发者模式”。
3. 点击“加载已解压的扩展程序”。
4. 选择项目目录：

   ```text
   browser-extension/leonardo-cookie-exporter
   ```

5. 确认扩展已启用。

如果插件已经安装过，修改插件代码后，在扩展管理页点击“重新加载”。

## 二、快速导出

每个账号完成 Leonardo 登录后，按以下步骤操作：

1. 打开 `https://app.leonardo.ai/`。
2. 等待页面完全加载，并确认能看到账号信息或积分余额。
3. 点击浏览器工具栏中的“Leonardo Cookie Exporter”。
4. 保持“包含全部 Leonardo Cookie”未勾选。
5. 点击“导出标准 JSON”，保存导出的 `.json` 文件。
6. 将该 JSON 文件导入 Leo-Go 的 Cookie 导入页面。

也可以点击“复制 Cookie 字符串”，直接粘贴到 Leo-Go 的 Cookie 导入框。

## 三、获取 Cookie 的核心代码

认证 Cookie 通常带有 `HttpOnly` 属性，不能使用网页控制台里的 `document.cookie` 获取。需要在 Chrome 扩展中使用 `chrome.cookies.getAll()`。

下面代码可以直接放到扩展的 `popup.js` 中使用：

```javascript
const SESSION_URL =
  "https://app.leonardo.ai/api/auth/get-session";

const AUTH_COOKIE_NAMES = new Set([
  "__Secure-better-auth.session_token",
  "CF_Access_Token",
]);

function isRequiredCookie(cookie) {
  return (
    AUTH_COOKIE_NAMES.has(cookie.name) ||
    cookie.name.startsWith("__Secure-better-auth.session_data.")
  );
}

function cookiePriority(cookie) {
  const exactHost = cookie.domain === "app.leonardo.ai" ? 100000 : 0;
  return exactHost + (cookie.path || "/").length;
}

function selectCookies(cookies) {
  const selected = new Map();

  for (const cookie of cookies.filter(isRequiredCookie)) {
    const current = selected.get(cookie.name);
    if (!current || cookiePriority(cookie) > cookiePriority(current)) {
      selected.set(cookie.name, cookie);
    }
  }

  return [...selected.values()].sort((a, b) => {
    const rank = (name) => {
      if (name === "__Secure-better-auth.session_token") return 0;
      if (name.startsWith("__Secure-better-auth.session_data.")) {
        return 10 + Number(name.split(".").pop() || 0);
      }
      if (name === "CF_Access_Token") return 100;
      return 1000;
    };
    return rank(a.name) - rank(b.name);
  });
}

async function getLeonardoCookie() {
  // url 参数只返回浏览器访问 get-session 时实际可发送的 Cookie。
  const cookies = await chrome.cookies.getAll({ url: SESSION_URL });
  const selected = selectCookies(cookies);

  if (!selected.some((cookie) =>
    cookie.name === "__Secure-better-auth.session_token"
  )) {
    throw new Error("未找到 Leonardo 登录 Cookie，请先登录");
  }

  const cookie = selected
    .map(({ name, value }) => `${name}=${value}`)
    .join("; ");

  return {
    cookie,
    cookie_count: selected.length,
    cookie_names: selected.map(({ name }) => name),
  };
}

getLeonardoCookie().then((result) => {
  // 不要打印 result.cookie，避免把完整凭据写入日志。
  console.log("Cookie 已获取", {
    cookie_count: result.cookie_count,
    cookie_names: result.cookie_names,
  });
});
```

扩展至少需要以下权限：

```json
{
  "permissions": ["cookies"],
  "host_permissions": ["https://app.leonardo.ai/*"]
}
```

## 四、正常导出的内容

通常至少应包含以下认证 Cookie：

```text
__Secure-better-auth.session_token
__Secure-better-auth.session_data.0
__Secure-better-auth.session_data.1
```

如果浏览器当前存在，也会包含：

```text
__Secure-better-auth.session_data.2
__Secure-better-auth.session_data.3
CF_Access_Token
```

`session_data` 是分片 Cookie，编号不一定固定。不要手动删除其中的分片，也不要只复制其中一个分片。

导出的 JSON 结构如下：

```json
{
  "cookie": "__Secure-better-auth.session_token=...; __Secure-better-auth.session_data.0=...; __Secure-better-auth.session_data.1=..."
}
```

## 五、导入 Leo-Go

### 管理后台导入

在 Leo-Go 管理后台打开 Cookie 导入功能，将 `.json` 文件上传，或粘贴 Cookie 字符串，然后等待后台验证完成。

导入成功后，系统会：

1. 调用 Leonardo `get-session` 获取新的短期 JWT。
2. 查询账号和积分信息。
3. 开启自动刷新（如果导入选项保持默认）。
4. 在官网轮换 Cookie 时保存新的 Cookie。

### API 批量导入

如果使用机器流程，可调用：

```http
POST /api/v1/tokens/import-cookie
Content-Type: application/json
X-Import-Key: <config.cookie_import_api_key>
```

请求示例：

```json
{
  "items": [
    {
      "name": "账号 001",
      "cookie": "__Secure-better-auth.session_token=...; __Secure-better-auth.session_data.0=...; __Secure-better-auth.session_data.1=..."
    }
  ],
  "auto_refresh": true
}
```

不要把真实 Cookie 写入日志、工单、聊天记录或代码仓库。

## 六、导出失败排查

### 插件提示没有读取到 Cookie

- 确认当前页面是 `app.leonardo.ai`，不是登录页或 `leonardo.ai` 营销页。
- 刷新 Leonardo 页面后，等待页面出现账号信息，再打开插件。
- 确认插件权限包含 `https://app.leonardo.ai/*`。
- 在 `chrome://extensions/` 点击插件的“重新加载”。

### 导入后立即刷新失败

- 重新打开 Leonardo 页面，确认账号仍处于登录状态。
- 重新导出，不要使用几小时前或几天前的旧文件。
- 保持“包含全部 Leonardo Cookie”未勾选；只有遇到 Cloudflare/Vercel 挑战时，才尝试勾选全部 Cookie 后重新导出。
- 确认服务器出口代理/IP 没有频繁变化。
- 不要同时高并发验证大量账号，建议并发控制在 1～2。

### 过一段时间后刷新失败

Leonardo 的 `accessToken` 通常只有约 1 小时有效期，但登录会话 Cookie 有更长有效期。Leo-Go 应在 JWT 到期前自动刷新，不需要重新导出 Cookie。

如果仍然失败，查看刷新失败原因：

- `401`：登录会话失效，需要重新登录并导出。
- `429`：触发限流，降低并发并等待后重试。
- `403` 或 HTML Challenge：通常是出口 IP、Cloudflare 或 Vercel 挑战，不一定代表 Cookie 失效。
- `timeout`、`EOF`、`5xx`：网络或上游临时故障，不要立即把账号标记为失效。

## 七、安全要求

Cookie 等同于登录凭据。导出后请遵守：

- 只传到自己的 Leo-Go 服务。
- 不要发送到第三方 Cookie 检测网站。
- 不要提交 Git、网盘公开链接或聊天群。
- 文件使用完后及时删除或加密保存。
- 如果 Cookie 泄露，立即在 Leonardo 退出登录并重新登录，使旧会话失效。
