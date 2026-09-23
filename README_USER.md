# NiniMenu 使用说明

> 一款温馨的每日菜单推荐与食谱管理应用，告别"今天吃什么"的纠结。

## 快速开始

1. 运行 `ninimenu.exe`（Windows）或 `./ninimenu`（Linux）
2. 浏览器打开 `http://localhost:8080`
3. 输入应用密码登录（默认 `nini123`）
4. 管理后台地址 `http://localhost:8080/admin`，默认密码 `nini123`

> 首次运行可在同目录下创建 `.env` 或 `env.bak` 文件修改端口、密码等配置。

---

## 功能介绍

### 智能推荐

- **午餐/晚餐推荐** — 按口味画像（常吃口味、菜系、收藏、喜恶、距上次食用天数）打分推荐，自动排除近期重复，并给出推荐理由
- **心情推荐** — 选择当前心情（开心、疲惫、想偷懒、想吃辣、想养生），获得匹配菜品
- **AI 推荐官** — 查看口味画像，按餐段/心情/口味/耗时/食材/忌口精确推荐，一键记入；接入智能体后还能对话点菜、查做法、排菜单
- **转盘抽奖** — 美食转盘随机选菜
- **盲盒惊喜** — 翻牌揭晓今日惊喜菜品
- **收藏加权** — 收藏的菜品推荐权重更高

### 明天吃什么

- 偏好过滤：快速/清淡/辣/收藏/均衡
- 设置目标道数，一键生成明日菜单
- 支持换菜、替换单道

### 食谱管理

- 菜单收录南方菜系 100 道（川菜、湘菜、贵州菜、云南菜、粤菜），以辣味为主
- 菜品信息：名称、图片/多图、视频、分类、口味
- 详细做法：食材、调料、步骤（支持计时）、烹饪时间、难度等级
- 烹饪模式：步骤引导，计时提醒
- 菜品启用/禁用、克隆、排序

### 周计划 & 购物清单

- 一键生成本周午餐/晚餐菜单
- 购物清单自动汇总食材，按蔬菜/肉类/配料/其他分类
- 支持勾选与库存标记

### 记录与回顾

- 用餐记录：记录每餐吃了什么，评分与备注
- 照片墙：时间线展示美食照片与心情
- 历史回顾：查看过往用餐记录

### 成就系统

- 80+ 成就自动解锁（初入厨房、连续七天、百菜斩、粤菜大师等）
- 按用餐/心情/推荐/照片/收藏/购物等维度自动检测
- 支持管理后台手动解锁

### 管理后台

- 仪表盘：菜品数、记录数、分类分布、热门菜品、周趋势
- 菜品 CRUD：搜索、筛选、分页、批量操作、多图上传
- 语录管理、成就管理、记录管理
- 系统设置：重复天数、每日菜品数、分类、口味、应用名称、AI 助手嵌入地址

### 开放给智能体

- 菜单、用餐记录、收藏、评价、行为事件、口味画像与推荐引擎通过 `/api/agent/*` 全量开放，凭证为 `AGENT_TOKEN`（默认与管理密码相同），供外部智能体做食谱推荐与行为分析

---

## 替换应用 Logo 图标

如需更换 NiniMenu 的应用 Logo（PWA 图标、浏览器图标等），请按以下流程操作：

### 步骤一：使用 GPT 生成菜品图片

使用 GPT 图像生成模型（如 `gpt-image-2`），生成 1:1 正方形菜品照片。

推荐 Prompt 模板：

```
生成菜品的照片 仅需要简单 突出菜品主题 色香味俱全食欲大增即可 菜品为[菜名] 将宽高比设为 1:1
```

项目内置了批量生成脚本（需配置 API Key）：

```bash
# 编辑 dish_images.txt 填入菜品名和输出路径，然后运行：
uv run python scripts/generate_dish_images_batch.py
```

也可单张生成：

```bash
uv run python scripts/generate_dish_image.py --prompt "菜品为青椒肉丝 将宽高比设为 1:1" --output output.png
```

菜品图片生成后通过管理后台上传即可，无需经过后续步骤。以下步骤二至步骤四仅适用于**制作和替换应用 Logo 图标**。

### 步骤二：使用 GPT 生成 Logo 图片

使用 GPT 图像生成模型生成 NiniMenu 品牌 Logo，推荐使用以下 Prompt：

```
根据上传照片中的女主人形象，设计一款「NiniMenu」品牌吉祥物 Logo。

NiniMenu 是一个温馨的家庭菜单与食谱管理应用，帮助解决“今天吃什么”的日常烦恼。

设计方向：

以女主人为绝对视觉中心，占整体画面 60%-70%。

将真实照片转化为高质量日系治愈风插画形象，保留黑色长发、温柔亲切、年轻自然的气质，展现热爱烹饪与照顾家庭的感觉。

围绕人物加入：

- 木勺
- 小奶锅
- 菜单便签
- 爱心
- 新鲜蔬菜
- 美食元素

整体采用圆形徽章构图（Badge Logo）。

风格参考：

- LINE FRIENDS
- Kakao Friends
- Disney Food Illustration
- 日本生活方式品牌
- 温馨家庭料理品牌

视觉关键词：

cute mascot logo,
food app logo,
family kitchen,
warm lifestyle,
healthy cooking,
sticker illustration,
high-end branding,
soft lighting,
premium mascot design

配色：

奶油白、珊瑚粉、暖橙色、浅棕色、鼠尾草绿。

要求：

- 品牌级 App Logo
- 高级插画质感
- 可爱但不幼稚
- 温暖治愈
- 高识别度
- 包含品牌名称「NiniMenu」
- 圆形徽章 Logo
- 1:1 构图
- 白色背景
- 超高清
Avoid minimalist icon logo, avoid flat corporate logo, avoid abstract symbol logo.

Focus on mascot badge logo with character illustration as the main subject.
```

> 生成时需上传女主人照片作为参考图。生成结果为白色背景的 PNG 图片。

### 步骤三：使用 imgtool.exe 去除白边并生成多尺寸图标

GPT 生成的 Logo 图片带有白色背景，需要使用 `backend\imgtool.exe` 处理：

```bash
# 基本用法：去除白边、裁剪到内容、生成多尺寸 PNG
backend\imgtool.exe logo.png

# 指定输出目录
backend\imgtool.exe -o output_folder logo.png

# 跳过裁剪（仅去白边，保留原始画幅）
backend\imgtool.exe -no-crop logo.png

# 仅生成多尺寸图标，不去白边、不裁剪
backend\imgtool.exe -no-trim logo.png

# 调整白边阈值（0-255，越高越严格，默认 240）
backend\imgtool.exe -t 250 logo.png
```

`imgtool.exe` 会自动完成：
1. 从边缘开始泛洪填充，将白色区域变为透明
2. 裁剪到非透明内容区域
3. 生成 6 种尺寸的 PNG 图标：**32×32、64×64、128×128、180×180、256×256、512×512**

### 步骤四：在线压缩图片

生成的多尺寸 PNG 文件可能较大，需要在线压缩：

1. 打开 [iloveimg 图片压缩](https://www.iloveimg.com/zh-cn/compress-image)
2. 上传步骤三生成的所有 PNG 文件
3. 使用默认压缩设置完成压缩
4. 下载压缩后的文件

### 步骤五：替换 static 中的图片文件

将压缩后的图片文件放入 `static/` 目录（对应 `backend/static/`），替换原有同名文件：

| 文件 | 用途 |
|------|------|
| `32.png` | 最小图标 |
| `64.png` | 小图标 |
| `128.png` | 中等图标 |
| `180.png` | Apple Touch Icon |
| `256.png` | 大图标 |
| `512.png` | PWA 启动图标 |

---

## 常见问题

**Q: 忘记密码怎么办？**

A: 在运行目录创建 `.env` 文件，设置 `APP_PASSWORD=新密码` 和 `ADMIN_PASSWORD=新密码`，重启即可。

**Q: 数据存在哪里？**

A: SQLite 数据库文件位于 `data/ninimenu.db`，备份此文件即可备份所有数据。

**Q: 如何修改端口？**

A: `.env` 中设置 `PORT=新端口号`，重启生效。

**Q: 手机可以用吗？**

A: 支持手机浏览器访问，支持 PWA 添加到主屏幕。
