# 视频文字提取方案

状态：已按本方案实现，原生运行链路已验证；实际部署版本以 `/healthz` 为准。核验日期：2026-09-28。配置与限制见 [视频做法提炼](video-recipes.md)。

## 采用方案

采用「公开字幕优先，缺字幕时本地语音转写」的流程，直接集成当前食谱项目。参考 video-transcript-workbench 的字幕读取和 SenseVoice 识别方式；界面、用户认证、任务进度、限额及食谱草稿继续由现有应用承载。

本地转写使用 `sherpa-onnx + SenseVoice Small int8`，通过随应用打包的薄 Python 脚本执行。Go 在有任务时启动子进程，结束后回收；FFmpeg、识别依赖和模型随当前应用一起部署，无需额外部署工作台或 ASR 服务。

这条路径不新增第三方字幕 / ASR API 费用，但会使用现有服务器的 CPU、内存、磁盘和带宽。提炼食谱继续使用现有文本模型，其费用与本地文字提取分开。

## 用户视频的实际验证

测试链接：`https://b23.tv/lnFjJz8`，对应 `BV1Vo1zB2EZf` 第 1 P，时长 210 秒。

- 当前播放器字幕接口此前未返回字幕文件。
- 参考工作台的 `/x/v2/dm/view` 读取方式，在生产服务器网络环境、不携带登录 Cookie 的情况下，返回 6 条字幕记录，包括 `ai-zh`。
- 已成功读取中文正文：158 段，合并后 1396 个字符（含换行）。内容以「这是一期黄焖辣子鸡的保姆级教程」开始，并包含「首先我们准备三斤鸡块」等做法描述。
- 因此，这条视频确实有当前服务器可读取的字幕，可以直接提取文字，不需要先做语音识别或画面 OCR。

字幕核验与语音核验分开进行：字幕接口已返回上述正文，同一视频的音轨也在受限容器内通过了本地识别。公开接口的本次成功不代表所有视频、所有平台或未来始终可用。

## 三个候选的结论

| 候选 | 已核实的能力 | 接入判断 |
| --- | --- | --- |
| [Bili2Text](https://github.com/lanbinleo/bili2text) | MIT；下载视频后用 Whisper / SenseVoice 转写；SenseVoice 安装项仍声明 `torch`；README 明确尚未优化服务器长期运行 | 可参考流程，整套安装不适合本项目当前资源 |
| [video-transcript-workbench](https://github.com/splexuan/video-transcript-workbench) | MIT；B 站字幕优先；`sherpa-onnx` 本地量化 SenseVoice；另有可选 faster-whisper；完整应用面向 Windows | 最适合参考字幕和本地识别部分，按现有 Go 应用边界接入 |
| [SnapAny](https://platform.snapany.com/zh) | 网页标明免费；开发者 API 按积分收费，首次赠送 50 积分；字幕提取 1 积分 / 次，语音转写 4 积分 / 分钟 | 不作为免费自动提取的依赖 |

SnapAny 的[免费 B 站字幕页面](https://snapany.com/zh/bilibili-subtitle-downloader)明确只提取已有字幕，不提供语音转写或画面 OCR；付费开发者 API 的语音转写是另一项能力。

核对的源码版本：Bili2Text `0248c84593bf66391dc2e38b4583ff72e3eae456`；video-transcript-workbench `4c548be4928b1181b58ca7c397217f3954dedb14`。

关键源码：

- [工作台 B 站字幕读取](https://github.com/splexuan/video-transcript-workbench/blob/4c548be4928b1181b58ca7c397217f3954dedb14/backend/app/infrastructure/bilibili.py#L71)
- [工作台 SenseVoice 转写](https://github.com/splexuan/video-transcript-workbench/blob/4c548be4928b1181b58ca7c397217f3954dedb14/backend/app/infrastructure/transcriber.py#L83)
- [工作台模型规格](https://github.com/splexuan/video-transcript-workbench/blob/4c548be4928b1181b58ca7c397217f3954dedb14/backend/app/infrastructure/model_catalog.py#L18)
- [Bili2Text 依赖声明](https://github.com/lanbinleo/bili2text/blob/0248c84593bf66391dc2e38b4583ff72e3eae456/pyproject.toml#L25)

## 接入流程

1. 用当前链接解析器校验分享文案，确认平台、视频 ID、分 P 和时长。
2. 优先读取已有字幕。补充已验证的 B 站字幕入口，使用目标分 P 的 CID，优先人工中文字幕，再选择中文 AI 字幕。仅在正常响应但无可用字幕时走下一条路径；平台拒绝或限流时按现有规则停止并冷却。
3. 无可读字幕时，从现有受限制的媒体读取器获取音频。FFmpeg 只处理已下载、受大小限制的本地文件，转为 16 kHz 单声道音频。
4. 调用本地 SenseVoice 分段转写，保留来源和时间段。不能将音频分段长度当作烹饪步骤用时。
5. 将文字交给现有食谱提炼和审核流程，返回可核对的草稿。用户确认保存后才写入菜谱。

平台抓取继续共用当前白名单、拨号 IP 校验、缓存、请求预算和冷却。子进程只能接收受控文件路径和固定模型配置，使用参数调用而非拼接 shell；取消、超时和失败均回收进程并清理临时音频。模型使用固定版本和哈希校验，复用第三方代码时保留许可声明。

代码和模型的许可分开核对：工作台代码是 MIT，sherpa-onnx 是 Apache 2.0；[原始 SenseVoiceSmall 模型卡](https://huggingface.co/FunAudioLLM/SenseVoiceSmall)指向独立的 [FunASR 模型协议](https://github.com/modelscope/FunASR/blob/main/MODEL_LICENSE)。该协议允许在遵守条款的前提下自由使用、复制、修改和分享，并要求保留来源、作者和模型名称，不能把工作台的 MIT 许可当作模型权重的许可。

## 资源与实现验证

当前服务器为 4 CPU，总内存约 3.3 GiB，核验时可用内存约 860 MiB、无可用 swap。SenseVoice 量化模型文件约 239 MB；这个数字是磁盘大小，不是推理峰值内存。

- 本地 ASR 全站同时只处理 1 个任务，短片段串行识别，限制线程数，任务结束释放模型。
- ASR 使用 2 线程、VAD 使用 1 线程；VAD 返回的片段可能超过配置值，应用再次切分，确保每次推理不超过 6 秒且不丢弃音频。开始时检查可用内存，运行期间监控 RSS，50 秒内不能完成则明确停止。
- 沿用 10 分钟 / 20 MiB 的输入上限，解码后也校验实际时长和大小。资源不足时返回可重试提示，不把失败结果当作完整文字。
- 运行层改用 Python 3.12 slim Debian，提供原生 wheel 所需的 glibc；Go 主程序保持 `CGO_ENABLED=0` 构建。没有加入 PyTorch、Whisper 服务或工作台。
- 实际 Go → Python → FFmpeg / SenseVoice 调用链已通过真实音频、取消、静音、损坏媒体、超长音轨和清理测试。首次完整识别约 16 秒，峰值受 512 MiB 上限保护；具体记录在发布 QA 文件中。
- 子进程增加 Landlock + seccomp，配置读写、联网、shell 执行和旧 Landlock 的截断绕过均有实际拒绝测试。宿主不支持沙箱时禁用本地 ASR，保留字幕与手动文稿入口。
- 完整验收还包括平台限流、双端草稿保护、用户视频生成食谱草稿，以及原有私房菜可见性和数据校验。

## 首版范围：字幕与语音

以上两个开源项目的所查文字提取实现均不包含 OCR。画面内文字如果没有对应字幕文件，也没有被解说念出来，语音识别无法获取它。

首版覆盖「字幕文件 + 语音解说」，纯画面字幕保留手动文稿入口。后续若增加 OCR，需要顺序处理画面以控制峰值内存，并专门验证字幕去重、漏帧及与语音冲突的用量；当前没有将画面里未念出的文字当作已识别内容。
