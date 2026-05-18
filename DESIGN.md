# Prism Design Language

Prism 的设计目标：**像一块被打磨过的棱镜**。克制、精致、有材质感，在浅色和深色主题下都应该看起来像"原生 macOS 应用"，而不是套壳网页。

本文档记录当前产品里已经落地的设计决策，任何新增页面 / 组件都应该对齐这些约束。

---

## 1. 核心原则

1. **低饱和、高对比**：主体配色使用中性灰阶，强调色只在"状态（success / warning / destructive）"和 focus ring 上出现。避免品牌色大面积铺底。
2. **材质优先于装饰**：用毛玻璃 (`backdrop-blur`)、细边 (`border/40`~`border/60`)、内阴影 (`shadow-[inset_…]`)、极淡的径向渐变来营造层次，不用插画、不用渐变大色块。
3. **边界要克制**：Prism 的卡片 / 表格 / 弹窗用 `border-border/40` ~ `border-border/60` 的半透明边，而不是实色边框；阴影同样取 `shadow-sm` / `shadow-2xl` 两档，中间不模糊。
4. **深色主题是主场，浅色主题要对等精致**：不能因为主场是暗色就把浅色模式降级成"灰底黑字"，浅色模式要专门调一遍对比度。
5. **信息密度适中**：按钮默认 `h-9`，输入框 `h-9`，表头 `h-10`，行内间距 `py-3`。全局统一，不要为了好看把某个页面做得更"松"或更"挤"。

---

## 2. 色彩系统

颜色全部使用 HSL CSS 变量，定义在 `frontend/src/styles.css`。**禁止在组件里硬编码十六进制颜色**，除了"终端 / 日志"专用的深色容器（见 §7）。

### 2.1 浅色主题 (`:root`)

| Token                      | HSL              | 用途                                 |
| -------------------------- | ---------------- | ------------------------------------ |
| `--background`             | `0 0% 100%`      | 应用最底层背景                       |
| `--foreground`             | `0 0% 9%`        | 主文字                               |
| `--card`                   | `0 0% 100%`      | 卡片 / 弹窗面板底                    |
| `--primary`                | `240 5.9% 10%`   | 主按钮 / 高强度标签                  |
| `--primary-foreground`     | `0 0% 98%`       | 主按钮文字                           |
| `--secondary` / `--muted`  | `240 5% 94%`     | 次级容器 / 静态背景                  |
| `--muted-foreground`       | `240 3.8% 42%`   | 次要文字（说明、描述、占位）         |
| `--border` / `--input`     | `240 5.9% 87%`   | 边框 / 输入框边                      |
| `--destructive`            | `0 72% 50%`      | 危险状态                             |
| `--ring`                   | `240 5.9% 10%`   | Focus ring                           |

### 2.2 深色主题 (`.dark`)

| Token                      | HSL            | 用途                                 |
| -------------------------- | -------------- | ------------------------------------ |
| `--background`             | `0 0% 4%`      | 近黑的底，留一点灰避免"死黑"         |
| `--card`                   | `0 0% 6%`      | 面板比底色稍高一档                   |
| `--primary`                | `0 0% 98%`     | 暗色下主按钮反色                     |
| `--muted`                  | `0 0% 12%`     | 次级容器                             |
| `--border`                 | `0 0% 14%`     | 非常柔的边                           |
| `--ring`                   | `0 0% 83.1%`   | Focus ring（浅色）                   |

### 2.3 状态色（绕开主题，深浅都适配）

| 状态         | 边框                         | 底                   | 文字                                      |
| ------------ | ---------------------------- | -------------------- | ----------------------------------------- |
| success      | `emerald-500/30`             | `emerald-500/10`     | `emerald-500` / dark `emerald-400`        |
| warning      | `amber-500/30`               | `amber-500/10`       | `amber-600` / dark `amber-400`            |
| destructive  | `destructive/30`             | `destructive/10`     | `destructive`                             |

---

## 3. 排版

- **字体栈**：`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif` + emoji fallback。不要打包任何自定义字体。
- **OpenType features**：`'cv02', 'cv03', 'cv04', 'cv11'`，开 SF Pro 的几个变体字形，数字更漂亮。
- **等级**：
  - 页面标题：`text-lg font-semibold tracking-tight`（顶栏）
  - 卡片标题：`text-lg font-semibold leading-none tracking-tight`
  - 正文：`text-sm`
  - 次要：`text-xs text-muted-foreground`
  - 表头：`text-xs font-medium uppercase tracking-wider text-muted-foreground`
- **长文 / 路径 / JSON**：用 `font-mono text-[11px]` 或 `font-mono text-xs`，并加 `break-all`，否则窗口缩窄时会横向出框。
- **选区色**：`selection:bg-primary/30`，主题无关地弱对比。

---

## 4. 间距与尺寸

| 项                          | 值                     |
| --------------------------- | ---------------------- |
| 基础圆角 `--radius`         | `0.75rem` (12px)       |
| 卡片 / 弹窗                 | `rounded-2xl`          |
| 按钮 / 输入 / select        | `rounded-lg`           |
| 徽章 / 小胶囊               | `rounded-full`         |
| 控件默认高度                | `h-9`                  |
| 表头高度                    | `h-10`                 |
| 单元格 padding              | `px-3 py-3`            |
| 页面 main padding           | `p-8` + `max-w-6xl`    |
| 侧边栏宽度                  | `w-[240px]`            |
| 顶栏高度                    | `h-[52px]`             |

**"页面内容居中"原则**：`main` 里用 `mx-auto max-w-6xl` 把内容控制在合理宽度，窗口放到 4K 也不会把单行撑到 2000px。

---

## 5. 材质与阴影

1. **毛玻璃面板**：Card、Dialog、Sidebar、Topbar、Table 外框都用 `backdrop-blur-*` + 半透明底。
   - Card: `bg-card/60 backdrop-blur-xl`
   - Table 外框: `bg-card/50 backdrop-blur-sm`
   - Dialog: `bg-background/95 backdrop-blur-xl`
   - DialogOverlay: `bg-black/40 backdrop-blur-sm`
   - Topbar: `bg-background/80 backdrop-blur-md`
   - Sidebar: `bg-white/70 supports-[backdrop-filter]:bg-white/55 backdrop-blur-xl` / `dark:bg-black/30 dark:supports-[backdrop-filter]:bg-black/20`
2. **阴影档位**（只能用这几档，不要自创）：
   - `shadow-sm`: 按钮默认、表格外框、Card。
   - `shadow-2xl`: 弹窗、悬浮菜单。
   - `shadow-[inset_0_0_0_1px_rgba(0,0,0,0.04)]` / `dark:shadow-[inset_0_0_0_1px_rgba(255,255,255,0.06)]`: 侧边栏当前选中项的内描边，模拟"按进去"的质感。
3. **全局背景氛围**：在 `App.tsx` 根部固定一层径向渐变装饰，不能去掉：
   ```
   bg-[radial-gradient(ellipse_80%_80%_at_50%_-20%,rgba(120,119,198,0.15),rgba(255,255,255,0))]
   ```
   强度极弱，只在大面积留白时提供"空间感"。

---

## 6. 组件规范

### 6.1 Button

来自 `frontend/src/components/ui/button.tsx`。6 个 variant：

| variant       | 用途                                  |
| ------------- | ------------------------------------- |
| `default`     | 主操作（保存、测试、启动）            |
| `secondary`   | 次级操作                              |
| `outline`     | 取消、辅助（半透明底 `bg-background/50`）|
| `destructive` | 删除、撤销                            |
| `ghost`       | 纯图标按钮 / 列表 hover 出来的操作    |
| `link`        | 行内文本型跳转                        |

- 所有按钮 `transition-all duration-200`，不要关动画。
- hover 时 `default` 按钮升阴影 (`hover:shadow`)，不是换颜色——颜色变化已经够弱。

### 6.2 Badge

`outline` 变体专门为浅色模式调过对比：`border-border bg-background text-foreground/80`。不要改回去。

状态徽章优先用 `success / warning / destructive`，**不要**用原色 `bg-green-500` 硬写。

### 6.3 Card

- 标题 padding `p-6`，内容 `p-6 pt-0`（标题和内容之间只靠 `space-y-1.5` 区隔）。
- 不要在卡片里再套卡片。需要分组时用 `border-t border-border/40` 的分割线。

### 6.4 Dialog

`DialogContent` 已经强制：
```
max-w-[calc(100vw-2rem)] max-h-[calc(100vh-4rem)] overflow-hidden
```
页面侧新增弹窗时**不要覆盖**这两条；超高内容用内部 `<div className="max-h-[70vh] overflow-auto">` 分区滚动。

右上关闭按钮始终存在，不要自己额外加一个"取消"关闭按钮混在头里——放到 `DialogFooter`。

### 6.5 Table

表格组件本身已经带：
- 外层 `rounded-xl border border-border/40 bg-card/50 shadow-sm backdrop-blur-sm overflow-auto`
- 行 hover `hover:bg-muted/40`

**禁止**再在 `<Table>` 外面套一层 `<div className="rounded-md border bg-card">`（会变成双层边框，已经清理过）。

长单元格（path / name / key）必须：
- `max-w-[…px] truncate`（按列类型选 160 / 180 / 220 / 280 / 360）
- `title={value}` 提供 hover 查看
- 对于 URL / JSON 展开区域改用 `break-all`

### 6.6 Input / Select

- 高度 `h-9`，圆角 `rounded-lg`，底色 `bg-background/50`（让毛玻璃透出来）。
- Focus 样式统一：`focus-visible:ring-1 focus-visible:ring-ring`。不要加 `ring-2` 或彩色 ring。

### 6.7 Sidebar

- 半透明毛玻璃，主题感知（见 §5）。
- 当前项样式：`bg-foreground/[0.06]` (light) / `dark:bg-white/10`，配合 inset shadow 模拟内凹。
- 非选中项：`text-muted-foreground`，图标 `opacity-60`，hover 时 `opacity-90` 并提升到 `text-foreground`。
- logo：`h-14 w-14 rounded-[16px]`，`shadow-[0_6px_20px_rgba(0,0,0,0.14)]` + `ring-1 ring-black/10 dark:ring-white/10`。**不要换成纯色占位符**。

### 6.8 Topbar

- 仅显示当前页面标题 + 语言开关 + 内核状态徽章。禁止在顶栏加业务按钮。
- 整个 header 设置 `--wails-draggable: drag`，右侧控件区 `no-drag`，保证可以拖动窗口。

---

## 7. 终端 / 日志 / JSON 展示

这是唯一允许脱离主题色系的区域。目的：让日志在浅色模式下也不会刺眼，保持"编辑器黑底"的习惯。

统一规范：
- 背景：`bg-[#0b0e14]`（硬编码，不要换成 `bg-muted`）。
- 边框：`border border-border/60`。
- 字体：`font-mono`，大小 `text-xs` 或 `text-[11px]`。
- 文字色：
  - 一般日志 / JSON：`text-slate-200`。
  - 事件流（tunnel 日志）：`text-emerald-200` / `text-green-200/90`。
  - 空状态提示：`text-white/40`。
- 长行必须 `whitespace-pre-wrap break-all`，避免 SSE 无空格单行撑破容器。
- 滚动条：统一使用 `.prism-scroll`（8px 细滚动条，在 `styles.css` 里定义）。

对应代码见 `frontend/src/pages/Logs.tsx` (行 319、533) 与 `frontend/src/pages/Tunnel.tsx` 的 log box。

---

## 8. macOS 原生化

来自 `main.go` 的窗口配置：

```go
Mac: &mac.Options{
    TitleBar:             mac.TitleBarHiddenInset(),
    Appearance:           mac.NSAppearanceNameDarkAqua,
    WebviewIsTransparent: true,
    WindowIsTranslucent:  true,
},
HideWindowOnClose: true,
```

对应前端要配合的约束：
1. **左上角红黄绿按钮区域**：Sidebar 顶部有 `<div className="h-8 w-full">` 占位，Topbar 高 52px，不要动这两个高度，否则按钮会压住内容。
2. **拖拽区域**：Sidebar logo 容器、Topbar 整个 header 都设置了 `--wails-draggable: drag`。新增卡片标题栏默认**不要**加 drag，只在真正希望拖动窗口的大面积留白处加。
3. **半透明支持**：Card / Dialog / Sidebar 都用 `bg-*/…` 而不是 `bg-*`。后期改版不要把透明度吃掉。
4. **关闭 = 隐藏**：`HideWindowOnClose: true`。意味着用户 `Cmd+W` 后进程还在后台跑（Cloudflare、代理、日志捕获）。文案中不要出现"关闭应用会停止服务"。真正退出只走 `Cmd+Q` 或 Dock 右键 Quit。

---

## 9. 图标与标识

- App icon 存放在 `build/appicon.png`（1024×1024），由 `crop.py` 生成。
  - 源图：用户提供的棱镜 PNG。
  - 裁剪中央 72% → 放到 88% 的画布上，保证 Dock 里视觉足够大。
  - 背景固定 `(6, 7, 11)`（近黑），不是纯黑。
- 前端侧边栏 logo：`frontend/src/assets/logo.png`，由同一脚本输出，保证应用内和 Dock 一致。
- Tray icon：`build/trayicon.png`（32×32），保留备用；目前未挂系统托盘。

**禁止**再用字母 "P" 做占位 logo。

---

## 10. 动画

- 组件级过渡统一 `transition-colors duration-150` 或 `transition-all duration-200`。
- 弹窗进入 / 退出：Radix 默认 `data-[state=open]:animate-in data-[state=closed]:animate-out`，**不要**自定义滑入方向。
- 禁止使用 framer-motion 做页面级花哨动画。Prism 是工具应用，不是 landing page。

---

## 11. 国际化

- 任何用户可见字符串必须走 `useI18n` 的 `t()`，不要硬编码中文或英文。
- 新增 key 同时写入 `zh` 和 `en` 两个字典（`frontend/src/lib/i18n.ts`）。
- 状态词（up / down / starting）已有统一 key，不要再造 `core.online` / `core.offline` 这类同义词。

---

## 12. 反模式清单

下面这些是已经踩过坑 / 明确不想再见到的写法：

- 在组件里硬编码 `#fff` / `#000` / `rgb(…)`（终端区域除外）。
- 把 Card 套在 Card 里。
- 给 `<Table>` 再额外包一层 `rounded-md border bg-card`。
- 用大色块渐变 `bg-gradient-to-r from-purple-500 to-pink-500`。
- Toast 位置放 `top-right`（挡住顶栏状态徽章）。已统一 `bottom-right`。
- 弹窗不给 `overflow` 限制，导致长 JSON / 长 URL 撑破窗口。
- 用 "P" 字母方块当 logo。
- 在浅色主题用 `bg-black/20` 想着"暗色看着还行"——浅色会变成灰墙。
- 自己加 `ring-2` 带彩色 focus 环。统一 `ring-1 ring-ring`。
- 直接 `sed` / `awk` 改样式。用组件 variant 或 `cn()` 合并 class。

---

## 13. 版本

- 本文档同步代码版本：`v0.1.0`
- 任何对 §2 色板、§4 尺寸、§5 材质、§8 macOS 配置的调整，都要同步更新本文件对应章节，不要只改代码。
