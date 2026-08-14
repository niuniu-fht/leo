# 🔥 Leo2API — Leonardo 视频生成 API 网关（Token 池自动调度）

> 把一堆 Leonardo 账号 Cookie 变成 **OpenAI 风格的一行 API**：视频生成反代 + Token 池自动调度 + JWT 自动保活 + 额度管理。
> 支持 Seedance 2.0 / Sora 2 / Kling O3 / MiniMax H3 等上游模型。

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/Go-1.23+-00ADD8.svg)
![PRs](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)
💬 交流群：[1101086346](https://qun.qq.com/qqun/1101086346)

> 🛒 **作者自营小店**：[Leonardo 17000 分账号 Cookie 直购（¥7/个，自动发货）](https://pay.ldxp.cn/item/oce5fc) —— 买回来配合 Leo2API 自动保活，导入即用，长期稳定在线。发货即 cookie，兑换后请尽快导入保活。

## ✨ 特性

- ✅ **OpenAI 风格接口**：`POST /v1/video/generations`、`POST /v1/images/generations` 一行接入，SDK/curl 直接替换 Base URL
- ✅ **多模型反代**：视频 `video-2.0` / `sora2` / `ko3` / `minimax-h3`，图片 `gpt-image-2` / `gpt-image-2-high` / `gpt-image-2-higher`
- ✅ **Token 池自动调度**：轮换策略（round_robin / random）、单 Token 并发上限、积分门槛跳过
- ✅ **自动保活**：JWT 到期前自动刷新，账号长期在线，无需人工干预
- ✅ **额度管理**：积分实时检测、额度耗尽自动禁用/清理、失败任务积分结算回补
- ✅ **失败重试**：提交阶段自动切换 Token，异步任务按错误类型复用原 Token 重试
- ✅ **远程素材**：图片 / 视频 / 音频参考自动下载并上传 Leonardo，支持首尾帧
- ✅ **管理后台**：Token 池管理、批量导入/导出、Cookie 导入、请求日志、代理检测
- ✅ **零依赖部署**：Go 单二进制 / Docker，SQLite 开箱即用（可选 Redis 扩展）

## 🏗️ 架构

```
┌──────────────┐   OpenAI 风格 API    ┌─────────────────────────┐
│   你的业务     │ ───────────────────▶ │         Leo2API           │
│  SDK / curl  │ ◀─────────────────── │  Token 池调度 / 轮换      │
└──────────────┘  /v1/video/...       │  JWT 自动保活刷新         │
                                     │  额度检测 / 失败重试        │
                                     └───────────┬─────────────┘
                                                 │ 上游生成请求
                                     ┌───────────▼─────────────┐
                                     │  Leonardo 账号池          │
                                     │  （浏览器扩展导出 Cookie   │
                                     │   → 后台批量导入）        │
                                     └─────────────────────────┘
```

## 目录

- [📸 界面预览](#-界面预览)
- [快速开始](#快速开始)
- [公共 API](#公共-api)
- [模型与能力](#模型与能力)
- [请求参数](#请求参数)
- [调用示例](#调用示例)
- [任务状态](#任务状态)
- [Leonardo 管理接口](#leonardo-管理接口)
- [Cookie 导入](#cookie-导入)
- [配置](#配置)
- [目录结构](#目录结构)
- [免责声明](#免责声明)

## 📸 界面预览

**管理后台登录**

![登录页](docs/screenshots/login.png)

**Token 号池管理**（状态徽章 / 自动保活开关 / 积分 / 有效期 / 批量操作）

![Token 管理](docs/screenshots/dashboard.png)

**请求日志**（统计卡片 + 生成记录，失败状态码可点击查看详情）

![请求日志](docs/screenshots/logs.png)

**系统配置**（代理 / 重试 / 自动保活 / 存储）

![系统配置](docs/screenshots/config.png)

## 快速开始

### 本地运行

```bash
go build -o leo2api.exe ./cmd/server/
./leo2api.exe
```

### Docker 运行

```bash
docker compose up -d
```

建议持久化：

- `/app/config`
- `/app/generated`

服务默认监听 `http://127.0.0.1:8787`。启动后可访问管理后台：

```text
http://127.0.0.1:8787/
```

管理员账号来自 `config/config.json`；未配置时默认用户名和密码均为 `admin`。

### 最小调用

```bash
curl -X POST http://127.0.0.1:8787/v1/video/generations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "video-2.0-fast",
    "prompt": "A cinematic drone shot over a neon city at dusk",
    "duration": 4,
    "size": "1280x720"
  }'
```

提交成功返回 `202 Accepted`。生成是异步的，需要使用响应中的 `poll_url` 查询结果。

图片生成：

```bash
curl -X POST http://127.0.0.1:8787/v1/images/generations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2-higher",
    "prompt": "A high detail product render of a translucent mechanical keyboard, studio lighting",
    "size": "1536x1536",
    "n": 1
  }'
```

图片接口同步等待上游完成，成功后返回 OpenAI 风格 `data[].url`。

## 公共 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/v1/models` | 查询支持的模型 |
| `POST` | `/v1/images/generations` | 同步生成图片，返回 OpenAI 风格 `data[].url` |
| `POST` | `/v1/video/generations` | 提交视频生成任务 |
| `GET` | `/v1/video/generations/{generation_id}` | 查询任务状态和结果 |
| `POST` | `/v1/video/async-generations` | 提交任务的兼容别名 |
| `GET` | `/v1/video/async-generations/{generation_id}` | 查询任务的兼容别名 |

所有公共 API 使用 Bearer API Key：

```http
Authorization: Bearer YOUR_API_KEY
```

查询模型：

```bash
curl http://127.0.0.1:8787/v1/models \
  -H "Authorization: Bearer YOUR_API_KEY"
```

## 模型与能力

### 模型总览

| 推荐模型名 | 上游模型 | 默认时长 | 默认尺寸 | 支持时长 | 参考能力 |
| --- | --- | ---: | --- | --- | --- |
| `gpt-image-2` | 配置项 `image_model_id*` | - | `1536x1536` | - | 图片生成，固定 `quality=low` |
| `gpt-image-2-high` | 配置项 `image_model_id*` | - | `1536x1536` | - | 图片生成，固定 `quality=medium` |
| `gpt-image-2-higher` | 配置项 `image_model_id*` | - | `1536x1536` | - | 图片生成，固定 `quality=high` |
| `gpt-image-2-clarity` | 配置项 `image_model_id*` + Adobe2API | - | `1536x1536` | - | 先用 `gpt-image-2` 低质量生图，再调用 Adobe2API 转透明底 |
| `video-2.0` | `seedance-2.0` | 10 秒 | `1280x720` | 4–15 秒 | 图片、首尾帧、视频、音频 |
| `video-2.0-fast` | `seedance-2.0-fast` | 10 秒 | `1280x720` | 4–15 秒 | 图片、首尾帧、视频、音频 |
| `video-2.0-mini` | `seedance-2.0-mini` | 10 秒 | `1280x720` | 4–15 秒 | 图片、首尾帧、视频、音频 |
| `video-2.0-480p` | `seedance-2.0` | 10 秒 | `864x496` | 4–15 秒 | 图片、首尾帧、视频、音频 |
| `video-2.0-fast-480p` | `seedance-2.0-fast` | 10 秒 | `864x496` | 4–15 秒 | 图片、首尾帧、视频、音频 |
| `video-2.0-mini-480p` | `seedance-2.0-mini` | 10 秒 | `864x496` | 4–15 秒 | 图片、首尾帧、视频、音频 |
| `sora2` | `sora-2` | 8 秒 | `720x1280` | 4、8、12 秒 | 文生视频、单首帧 |
| `ko3` | `kling-video-o-3` | 3 秒 | `1080x1920` | 3–15 秒 | 图片、首尾帧、视频 |
| `minimax-h3` | `hailuo-03` | 5 秒 | `2560x1440` | 5–15 秒 | 图片、首尾帧、图片加音频 |

`size` 的格式统一为 `宽x高`。

图片模型的质量由服务端按 `model` 固定映射：`gpt-image-2 -> low`，`gpt-image-2-high -> medium`，`gpt-image-2-higher -> high`，`gpt-image-2-clarity -> low + Adobe2API 透明底后处理`。请求体里的 `quality` 字段会被忽略。

### 模型别名

建议新接入只使用上表中的推荐模型名。以下名称用于兼容旧调用或上游名称：

| 推荐模型名 | 兼容别名 |
| --- | --- |
| `video-2.0` | `seedance-2.0` |
| `video-2.0-fast` | `seedance-2.0-fast` |
| `video-2.0-mini` | `seedance-2.0-mini` |
| `video-2.0-480p` | `seedance-2.0-480p` |
| `video-2.0-fast-480p` | `seedance-2.0-fast-480p` |
| `video-2.0-mini-480p` | `seedance-2.0-mini-480p` |
| `sora2` | `sora-2` |
| `ko3` | `kling-o3`、`kling-video-o-3` |

MiniMax H3 的公共 API 请求、响应和请求日志统一使用 `minimax-h3`；不接受 `h3` 或 `hailuo-03` 作为公共请求模型名。`hailuo-03` 仅是服务调用 Leonardo 时的内部上游映射。

### 尺寸与比例

#### 图片生成

| `aspect_ratio` | `size` |
| --- | --- |
| `1:1` | `1536x1536` |
| `16:9` | `2752x1536` |
| `9:16` | `1536x2752` |
| `4:3` | `2048x1536` |

也可直接传 `size`，例如 `1024x1024`、`1536x1536`、`2752x1536`、`1536x2752`、`2048x1536`。

#### Video 2.0

标准版本：

| 比例 | 尺寸 |
| --- | --- |
| `16:9` | `1280x720` |
| `9:16` | `720x1280` |
| `1:1` | `960x960` |

480p 版本：

| 比例 | 尺寸 |
| --- | --- |
| `16:9` | `864x496` |
| `9:16` | `496x864` |
| `1:1` | `640x640` |

#### Sora 2

- `1280x720`（16:9）
- `720x1280`（9:16）

#### Kling O3

- `1920x1080`（16:9）
- `1080x1920`（9:16）
- `1440x1440`（1:1）

使用视频参考且未传尺寸时，`ko3` 会按上游格式发送 `width=0`、`height=0`，默认时长为 5 秒。

#### MiniMax H3

MiniMax H3 默认使用 2K。`size` 和 Leonardo 上游参数均按输出视频的“宽x高”填写：

| `aspect_ratio` | `size` | 上游参数 |
| --- | --- | --- |
| `16:9` | `2560x1440` | `width=2560, height=1440` |
| `9:16` | `1440x2560` | `width=1440, height=2560` |
| `1:1` | `1440x1440` | `width=1440, height=1440` |
| `4:3` | `1920x1440` | `width=1920, height=1440` |
| `3:4` | `1440x1920` | `width=1440, height=1920` |
| `21:9` | `3360x1440` | `width=3360, height=1440` |

### MiniMax H3 模式规则

- 不传图片时为文生视频。
- `image_url`、`image_urls` 或 `image_guidance` 均为图片参考模式。
- 即使只上传一张图片，也使用图片参考模式，不会自动当作首帧。
- 图片参考最多 5 张，默认 `strength=MID`。
- 只有明确传入 `start_image_url`、`start_frame`、`end_image_url` 或 `end_frame` 时才进入首尾帧模式。
- 图片参考模式与首尾帧模式不能混用。
- 音频参考必须和至少一张图片参考一起使用。
- 首尾帧模式不支持音频参考。
- 不支持视频参考。
- 上游请求固定使用 `model=hailuo-03`、`quantity=1`、`motion_has_audio=true`，不发送 `mode`、`seed` 或 `prompt_enhance`。

### Token 积分门槛

已配置固定调度门槛的模型如下：

| 模型或模式 | 最低剩余积分 |
| --- | ---: |
| `video-2.0` | 4550 |
| `video-2.0-fast` | 3650 |
| `video-2.0-mini` | 2400 |
| `video-2.0-480p` | 2150 |
| `video-2.0-fast-480p` | 1700 |
| `video-2.0-mini-480p` | 1200 |
| `sora2` | 1200 |
| `ko3` 文生、图片、首尾帧 | 4200 |
| `ko3` 视频参考或混合视频参考 | 3400 |
| `minimax-h3` | 2100 |

积分低于当前模型门槛时只跳过该 Token；低于 1200 才标记为额度耗尽并关闭自动刷新。

## 请求参数

### 通用字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `prompt` | string | 是 | 提示词，至少 3 个字符 |
| `model` | string | 否 | 默认 `video-2.0-fast` |
| `duration` | integer | 否 | 生成视频时长，取值范围由模型决定 |
| `size` | string | 否 | 输出尺寸，格式为 `宽x高` |
| `aspect_ratio` | string | 否 | 按模型映射为输出尺寸 |
| `width` | integer | 否 | 显式输出宽度 |
| `height` | integer | 否 | 显式输出高度 |
| `async` | boolean | 否 | 兼容字段；接口始终异步提交 |

尺寸覆盖优先级为：`width` / `height` 高于 `size`，`size` 高于 `aspect_ratio`，最后才使用模型默认值。

### 图片参考

| 字段 | 用途 |
| --- | --- |
| `image_url` | 单张远程图片 |
| `image_urls` | 多张远程图片 URL 数组 |
| `image_guidance` | 图片对象数组，可传 `url`、`strength` |

远程图片会先上传到 Leonardo，再转换为 `guidances.image_reference`。

图片生成接口支持 `image_url` / `image_urls` 作为图生图参考，最多取前 4 张，默认参考强度 `MID`。

除 `sora2` 外，`image_url` 默认都是图片参考；`sora2` 为兼容旧格式，会把 `image_url` 当作首帧。

### 首尾帧字段

| 字段 | 用途 |
| --- | --- |
| `start_image_url` | 远程首帧图片 |
| `end_image_url` | 远程尾帧图片 |
| `start_frame` | 首帧对象数组，传入远程 `url` |
| `end_frame` | 尾帧对象数组，传入远程 `url` |

### 视频参考字段

| 字段 | 用途 |
| --- | --- |
| `video_url` | 单个远程 MP4 |
| `video_reference` | 视频对象数组，可传 `url`、`duration` |

顶层 `duration` 是生成结果时长，`video_reference[].duration` 是参考视频本身的时长。

对于远程视频，服务会下载素材、上传到 Leonardo、等待 `uploaded_media.status=COMPLETE`，并尽量自动读取源视频时长。上游要求参考视频宽高处于 720px–2160px 范围内；不符合时需先转码、缩放或补边。

### 音频参考

| 字段 | 用途 |
| --- | --- |
| `audio_url` | 单个远程音频 |
| `audio_reference` | 音频对象数组，可传 `url`、`duration` |

远程音频支持 `mp3`、`wav`、`m4a`、`aac`、`ogg`、`webm`。服务会上传音频、等待素材就绪，并尽量读取参考音频时长。

## 调用示例

以下示例只展示请求 JSON，统一提交到：

```text
POST /v1/video/generations
```

图片生成示例提交到：

```text
POST /v1/images/generations
```

### 文生图

```json
{
  "model": "gpt-image-2",
  "prompt": "一只玻璃质感的白猫坐在赛博朋克雨夜街角",
  "size": "1536x1536",
  "n": 1
}
```

### 高质量图片

```json
{
  "model": "gpt-image-2-higher",
  "prompt": "Ultra detailed cinematic concept art, mountain temple above clouds",
  "aspect_ratio": "16:9",
  "n": 1
}
```

### 图生图

```json
{
  "model": "gpt-image-2-high",
  "prompt": "保持主体姿态，改成高级商业海报风格",
  "image_url": "https://example.com/source.png",
  "size": "1536x1536"
}
```

### 文生视频

```json
{
  "model": "minimax-h3",
  "prompt": "龟兔赛跑",
  "duration": 5,
  "size": "2560x1440"
}
```

### 单图参考

```json
{
  "model": "minimax-h3",
  "prompt": "动物世界",
  "duration": 5,
  "size": "2560x1440",
  "image_url": "https://example.com/animal.png"
}
```

单张图片仍会转换为 `guidances.image_reference`。

### 多图参考

简单写法：

```json
{
  "model": "minimax-h3",
  "prompt": "动物世界",
  "duration": 5,
  "image_urls": [
    "https://example.com/a.png",
    "https://example.com/b.png",
    "https://example.com/c.png"
  ]
}
```

显式写法：

```json
{
  "model": "video-2.0-fast",
  "prompt": "图一和图二的人物出现在图三的场景里",
  "duration": 4,
  "size": "720x1280",
  "image_guidance": [
    {"url": "https://example.com/character-1.png", "strength": "MID"},
    {"url": "https://example.com/character-2.png", "strength": "MID"},
    {"url": "https://example.com/scene.png", "strength": "MID"}
  ]
}
```

### 首尾帧

```json
{
  "model": "minimax-h3",
  "prompt": "恐龙变成兔子",
  "duration": 5,
  "size": "2560x1440",
  "start_image_url": "https://example.com/start.png",
  "end_image_url": "https://example.com/end.png"
}
```

### 图片加音频

```json
{
  "model": "minimax-h3",
  "prompt": "恐龙冒险",
  "duration": 5,
  "size": "1440x2560",
  "image_url": "https://example.com/dinosaur.png",
  "audio_url": "https://example.com/adventure.mp3"
}
```

### 视频参考

`ko3` 使用视频参考时建议省略 `duration` 和 `size`，由服务采用抓包默认值：

```json
{
  "model": "ko3",
  "prompt": "把视频中的香水替换成牙膏",
  "video_url": "https://example.com/source.mp4"
}
```

### 图片加视频

```json
{
  "model": "ko3",
  "prompt": "把视频中的主体替换为参考图中的小熊",
  "image_url": "https://example.com/bear.png",
  "video_url": "https://example.com/source.mp4"
}
```

多图或多视频时，分别使用 `image_urls` / `image_guidance` 和 `video_reference` 对象数组；`video_reference` 不接受纯字符串数组。

### Sora 2 首帧

```json
{
  "model": "sora2",
  "prompt": "武侠电影镜头",
  "duration": 8,
  "size": "1280x720",
  "image_url": "https://example.com/start.png"
}
```

`sora2` 最多接收一张首帧图片，不支持尾帧、图片参考、视频参考或音频参考。

## 任务状态

### 提交响应

```json
{
  "id": "1f149999-aaaa-bbbb-cccc-1234567890ab",
  "object": "video.generation",
  "created": 1770000000,
  "model": "minimax-h3",
  "status": "in_progress",
  "poll_url": "/v1/video/generations/1f149999-aaaa-bbbb-cccc-1234567890ab",
  "request_id": "1f149999-aaaa-bbbb-cccc-1234567890ab"
}
```

查询：

```bash
curl http://127.0.0.1:8787/v1/video/generations/1f149999-aaaa-bbbb-cccc-1234567890ab \
  -H "Authorization: Bearer YOUR_API_KEY"
```

成功响应：

```json
{
  "id": "1f149999-aaaa-bbbb-cccc-1234567890ab",
  "object": "video.generation",
  "created": 1770000000,
  "model": "minimax-h3",
  "status": "succeeded",
  "request_id": "1f149999-aaaa-bbbb-cccc-1234567890ab",
  "data": [
    {"url": "https://example.com/final.mp4"}
  ]
}
```

失败响应：

```json
{
  "id": "1f149999-aaaa-bbbb-cccc-1234567890ab",
  "object": "video.generation",
  "created": 1770000000,
  "model": "minimax-h3",
  "status": "failed",
  "request_id": "1f149999-aaaa-bbbb-cccc-1234567890ab",
  "error": {
    "message": "Generation failed in Leonardo",
    "type": "server_error"
  }
}
```

## Leonardo 管理接口

这些接口面向管理端，调用时需要管理员身份。生成接口中的图片、视频和音频引导字段可直接传远程 `url`。

### 上传图片

```bash
curl -X POST http://127.0.0.1:8787/api/v1/leonardo/upload-image \
  -F "file=@/path/to/image.jpg" \
  -F "token_id=YOUR_TOKEN_ID"
```

### 上传音频

```bash
curl -X POST http://127.0.0.1:8787/api/v1/leonardo/upload-audio \
  -F "file=@/path/to/reference.mp3" \
  -F "token_id=YOUR_TOKEN_ID"
```

### 提交任务

```bash
curl -X POST http://127.0.0.1:8787/api/v1/leonardo/generate \
  -H "Content-Type: application/json" \
  -d '{
    "token_id": "YOUR_TOKEN_ID",
    "model": "minimax-h3",
    "prompt": "A cinematic animal adventure",
    "public": true,
    "duration": 5,
    "width": 1440,
    "height": 2560,
    "image_guidance": [
      {"url": "https://example.com/animal.png", "strength": "MID"}
    ]
  }'
```

### 查询任务

```bash
curl "http://127.0.0.1:8787/api/v1/leonardo/status?id=GENERATION_ID&token_id=YOUR_TOKEN_ID"
```

## Cookie 导入

自动化程序可通过独立导入密钥把 Leonardo Cookie 写入 Token 池，无需管理员账号密码。

在管理后台的 `系统配置 -> 账号与安全 -> Token 池导入密钥` 设置导入密钥，对应配置项为 `cookie_import_api_key`。

接口：

```text
POST /api/v1/tokens/import-cookie
```

支持两种鉴权方式：

```http
Authorization: Bearer YOUR_COOKIE_IMPORT_API_KEY
```

或：

```http
X-Import-Key: YOUR_COOKIE_IMPORT_API_KEY
```

### 单个导入

```bash
curl -X POST http://127.0.0.1:8787/api/v1/tokens/import-cookie \
  -H "Authorization: Bearer YOUR_COOKIE_IMPORT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "account@example.com",
    "cookie": "__Secure-better-auth.session_token=...; ..."
  }'
```

### 批量导入

```json
{
  "items": [
    {
      "name": "account-a@example.com",
      "cookie": "__Secure-better-auth.session_token=...; ..."
    },
    {
      "name": "account-b@example.com",
      "cookie": "__Secure-better-auth.session_token=...; ..."
    }
  ]
}
```

也支持只传 Cookie 字符串：

```json
{
  "cookies": [
    "__Secure-better-auth.session_token=...; ...",
    "__Secure-better-auth.session_token=...; ..."
  ]
}
```

批量导入不限制条数。导入后会校验 Cookie，更新账号邮箱、积分和过期时间，并默认开启 `auto_refresh`。识别到相同 Leonardo 账号时会覆盖旧 Cookie，避免重复记录。接口不会回显原始 Cookie。

响应示例：

```json
{
  "ok": true,
  "total": 1,
  "success_count": 1,
  "failed_count": 0,
  "duplicate_count": 0,
  "overwritten_count": 0,
  "items": [
    {
      "index": 0,
      "status": "active",
      "detail": "imported",
      "token_id": "5UNuopWd",
      "token_account_email": "account@example.com",
      "credits": 8500,
      "overwritten": false,
      "duplicate": false
    }
  ]
}
```

## 配置

默认配置文件：

```text
config/config.json
```

最小示例：

```json
{
  "admin_username": "admin",
  "admin_password": "admin",
  "api_key": "",
  "proxy": "",
  "use_proxy": false,
  "image_request_model": "nano-banana-2",
  "image_request_model_banana2": "gpt-image-gemini-3.1-flash-image",
  "image_request_model_bananapro": "gpt-image-gemini-3-pro-image",
  "image_model_id": "",
  "image_model_id_gpt_image_2": "",
  "image_model_id_gpt_image_2_high": "",
  "image_model_id_gpt_image_2_higher": "",
  "gpt_image_size_mode": "request",
  "generate_timeout": 300,
  "retry_enabled": true,
  "retry_max_attempts": 3,
  "retry_same_token_error_types": ["provider_moderation_error"],
  "token_rotation_strategy": "round_robin",
  "token_max_running_tasks": 2,
  "exhausted_token_auto_cleanup_enabled": false,
  "exhausted_token_auto_cleanup_interval_hours": 24
}
```

### 图片模型配置

图片生成走 Leonardo `Generate` GraphQL，公共模型 ID 与质量固定如下：

| 公共模型 | 上游 `quality` |
| --- | --- |
| `gpt-image-2` | `low` |
| `gpt-image-2-high` | `medium` |
| `gpt-image-2-higher` | `high` |

上游图片模型 UUID 可通过配置映射：

| 配置项 | 作用 |
| --- | --- |
| `image_request_model` | 未知图片模型的 Leonardo `request.model`，默认 `nano-banana-2`；`banana2` 默认 `gpt-image-gemini-3.1-flash-image`，`bananapro` 默认 `gpt-image-gemini-3-pro-image` |
| `image_model_id` | 三个公共图片模型共用的默认上游 `modelId` |
| `image_model_id_gpt_image_2` | 覆盖 `gpt-image-2` 使用的上游 `modelId` |
| `image_model_id_gpt_image_2_high` | 覆盖 `gpt-image-2-high` 使用的上游 `modelId` |
| `image_model_id_gpt_image_2_higher` | 覆盖 `gpt-image-2-higher` 使用的上游 `modelId` |
| `gpt_image_size_mode` | GPT 图片尺寸处理：`request` 按请求，`1k` 按最接近比例映射到 1k 预设尺寸，无法识别的 size 使用 `1024x1024` |

如果三个质量档使用同一个上游模型，只填 `image_model_id` 或 `image_model_id_gpt_image_2` 即可；如果每档不同，就分别填写三个覆盖项。

### Token 轮换

`token_rotation_strategy` 支持：

| 值 | 行为 |
| --- | --- |
| `round_robin` | 从上次选中位置的下一个 Token 继续 |
| `round_robin_from_start` | 每个任务都从最早导入的可用 Token 开始扫描 |
| `random` | 随机打乱可用 Token |

`token_max_running_tasks` 控制单个 Token 同时运行的生成任务数，默认 2，范围 1–10。可在管理后台的 `系统配置 -> 刷新与存储` 修改。

### 额度耗尽自动清理

开启 `exhausted_token_auto_cleanup_enabled` 后，后台会按 `exhausted_token_auto_cleanup_interval_hours` 的小时间隔删除状态为 `exhausted` 的 Token。

正在准备任务、生成中或等待失败任务积分结算的 Token 会被跳过，待任务结束后再由下一轮清理。

## 目录结构

```text
Leo2API/
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── handler/
│   ├── provider/leonardo/
│   ├── reqlog/
│   ├── store/
│   └── token/
├── static/
├── config/config.json
├── Dockerfile
└── docker-compose.yml
```

## 免责声明

本项目仅用于技术学习与交流。使用本项目时请遵守：

- 上游平台（Leonardo 及对应模型厂商）的**服务条款**与当地法律法规。
- Cookie 等同于登录凭据，请妥善保管：只上传到自己的服务，不要提交到公开仓库、聊天群或第三方检测网站。
- 本项目不提供任何账号获取、注册自动化相关能力；账号来源与合规性由使用者自行负责。
- 使用本项目产生的任何后果（账号封禁、费用、法律风险等）由使用者自行承担。

## License

[MIT](LICENSE)

---

⭐ **点个 Star 就是最大的支持！**

💬 遇到问题欢迎进交流群讨论：**1101086346**（[点击加群](https://qun.qq.com/qqun/1101086346)）

🛒 需要账号？**作者自营小店**：[Leonardo 17000 分 Cookie ¥7 直购](https://pay.ldxp.cn/item/oce5fc)，自动发货，配合 Leo2API 保活即用。
