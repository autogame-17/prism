# e2e-sandbox

通用的 Linux GUI 应用 e2e 沙盒。本目录提供一套与具体项目无关的镜像 +
键鼠注入 + VNC 反馈基础设施，这次首先用于 prism-desktop，但同样可以挪给
其他任何 Linux GUI 项目（Electron、原生 GTK/Qt、Wails 等）。

## 为什么不直接在宿主机跑

prism-desktop 是 Wails (macOS .app) 应用，但我们的 e2e 不能依赖 macOS
原生 GUI——AI agent 没有可靠的 macOS 键鼠注入通道。Linux 容器可以跑
Xvfb + xdotool 组合，键鼠由命令行驱动，画面用 VNC 看，干净又通用。

容器与宿主完全隔离：

- 镜像不读宿主任何用户数据；只 bind mount 了仓库代码到 `/workspace`。
- Wails 应用配置写到容器内 `/tmp/prism-config`、`/tmp/prism-data`
  （由 `launchers/prism.sh` 重定向 `XDG_*`），随 `docker rm` 一起消失。
- 端口映射只绑 `127.0.0.1`，noVNC 不会被局域网访问。

## 组成

| 文件 | 职责 |
|---|---|
| `Dockerfile` | 通用基底镜像：Ubuntu 24.04 + Xvfb + x11vnc + noVNC + xdotool + Wails 构建依赖 + Go + Node。 |
| `docker-entrypoint.sh` | 启动 GUI 栈（Xvfb / fluxbox / dbus / x11vnc / noVNC），最后 exec launcher。 |
| `launchers/prism.sh` | **项目特定**：首次进入容器时构建 Linux 版 Prism 并运行。其他项目替换此文件即可复用整套基础设施。 |
| `scripts/run-e2e.sh` | 主入口（`start` / `exec` / `logs` / `stop`）。 |

## 使用流程（prism-desktop）

```bash
# 1) 启动沙盒（首次会构建镜像 + Wails Linux build，约 5-10min）。
e2e-sandbox/scripts/run-e2e.sh start

# 控制台会打印 noVNC URL，比如 http://127.0.0.1:6080/vnc.html?...
# 在任意浏览器打开就能看到 Prism 实时画面。

# 2) 模拟键鼠：
e2e-sandbox/scripts/run-e2e.sh exec xdotool search --name 'Prism' windowactivate
e2e-sandbox/scripts/run-e2e.sh exec xdotool key Return
e2e-sandbox/scripts/run-e2e.sh exec xdotool type --delay 50 'hello'
e2e-sandbox/scripts/run-e2e.sh exec xdotool mousemove 600 400 click 1

# 3) 直接读容器内 sqlite 验证落库：
e2e-sandbox/scripts/run-e2e.sh exec sqlite3 \
    /tmp/prism-data/Prism/prism.db \
    "select id,request_id,is_stream,finished,chunk_count from prism_traces"

# 4) 通过容器内 gin server 发模拟流量（不出容器）：
e2e-sandbox/scripts/run-e2e.sh exec curl -N -s \
    http://127.0.0.1:39527/v1/chat/completions \
    -H 'Authorization: Bearer xxx' \
    -d '{"model":"...","stream":true,"messages":[...]}'

# 5) 关停 + 清理：
e2e-sandbox/scripts/run-e2e.sh stop
```

## 复用到其他 Linux GUI 项目

1. 把 `e2e-sandbox/` 这一整个目录复制到新仓库根。
2. 把 `launchers/prism.sh` 改成你项目的启动脚本（构建 + 运行）。
3. 如果你项目不依赖 Wails / WebKit，可以在 `Dockerfile` 里精简掉
   `libgtk-3-dev libwebkit2gtk-4.1-dev` 那段。
4. `scripts/run-e2e.sh` 里把 `IMAGE / CONTAINER / GIN_PORT` 三个变量
   按你项目改名即可。
5. xdotool / VNC / Xvfb / dbus 这一套 GUI 基础设施完全无需修改。

## 已知限制

- 容器是 linux/aarch64 或 linux/amd64（取决于宿主），所以应用必须能在
  Linux 上构建。macOS 原生 .app 没法直接跑（这是有意为之，参见上文）。
- Wails 在 Linux 上只支持 WebKit2GTK，不是 macOS 原生 WKWebView，
  渲染细节可能与 macOS 真机有 1-2px 误差。功能性测试不受影响，但像素
  级回归请仍然在真机做。
- 浏览器 MCP 通过 noVNC 看到画面后，**主要应当用 xdotool 注入键鼠
  事件**——browser_mouse_click_xy 点击的是 noVNC 网页，不是容器内
  Prism 窗口的坐标系，会错位。
