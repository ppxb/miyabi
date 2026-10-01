# FANZA 来源验证

验证日期：2026-10-01。结论：FANZA 数字视频的 GraphQL 检索、详情、演员作品列表和图片下载通过有限样本验证，并已按用户确认正式接入为首选刮削来源；JavDB 为最终兜底。预告片播放、旧商品覆盖和长期稳定性仍有未通过的范围，不以接入完成宣称全部覆盖。

## 验证方式

- 分别直连和使用用户提供的 `http://127.0.0.1:10777` 代理，不修改应用或系统代理配置。
- 全程使用 HTTP 请求，无浏览器操作，不访问 JavDB，也不依赖 JavDB ID、账号或 API key。
- 参考 MetaTube SDK `19a92ad3263ab7b69636d26d7cc32f531c0a0804` 的 FANZA 详情查询及测试商品，再读取 FANZA 当日官方前端的 GraphQL 搜索定义。
- 原始响应、官方脚本、真实图片和复现脚本放在 `%TEMP%/miyabi-fanza-verification/`，不纳入 Git。`verify.ps1 -Mode direct/proxy` 可复验样本；`probe.go` 额外验证 Go 标准 HTTP 客户端与 JPEG 完整解码，未安装依赖。
- 下表图片尺寸为实际下载、解码的结果，不根据 URL、字段名或文件大小推测。每个有详情的商品验证封面及前两张预览图，两种连接方式各覆盖 15 张图片；素人封面补测了 mediumUrl 回退。

## 检索和详情接口

入口均为 `POST https://api.video.dmm.co.jp/graphql`。请求头使用 `Content-Type: application/json`、`Referer: https://video.dmm.co.jp/` 和 `Fanza-Device: BROWSER`。

MetaTube 所用的统一 HTML 搜索入口在两种连接下均重定向到地区限制页。官方数字视频前端使用的 `legacySearchPPV` 在相同网络下可用，因此无需依赖这个 HTML 搜索入口，也无需执行页面脚本。

本轮使用的最小检索：

```graphql
query MovieSearch($keyword: String!, $floor: PPVFloor, $offset: Int!) {
  legacySearchPPV(
    limit: 40, offset: $offset, floor: $floor, sort: RECOMMENDED,
    queryWord: $keyword, includeExplicit: true, excludeUndelivered: false
  ) {
    result {
      contents { id title packageImage { largeUrl mediumUrl } }
      pageInfo { offset limit hasNext totalCount }
    }
  }
}
```

`ppvContent(id: ...)` 读取详情，以 `makerContentId` 校验官方番号。`reviewSummary(contentId: ...)` 提供评分及评价数量。存在 HTTP 200 但详情为 null 的情况，必须同时检查 HTTP 状态、GraphQL errors 和业务内容。

### 番号检索边界

| 输入 | 实际结果 | 接入约束 |
| --- | --- | --- |
| `SSIS-001` | 0 条 | 不能原样发送带连字符的番号就判定未收录 |
| `ssis00001` / `ssis00001#` | 唯一 `ssis00001` | 此样本支持数字视频检索格式 |
| `ssis001` | 73 条相近商品，第一页 40 条 | 不能取第一条；需要官方番号校验及分页/候选数量边界 |
| `fuyu00079` | 0 条 | 数字补零不是通用内容 ID 规则 |
| `fuyu079` | 唯一 `fuyu079` | 紧凑格式可作为另一检索候选，不能覆盖更强身份匹配 |
| `stars00141`、`stars141`、`1stars00141` 及带 `#` 变体 | 0 条，但已知 ID 有详情 | 搜索可见性与详情可读取性不同；不能宣称覆盖全部历史商品 |

接入时由少量通用检索格式产生候选，再核对官方番号。禁止维护厂商数字前缀猜测表或为上述单片增加特例。返回空结果时不能选择相似影片代替。

## 真实样本结果

直连与代理下的业务结果一致。图片分辨率不因使用代理而变化。

| 请求番号 / 内容 ID | 番号检索 | 已知 ID 详情 | 封面实测 | 返回预览图数 |
| --- | --- | --- | --- | --- |
| SSIS-001 / `ssis00001` | 唯一命中 | 成功 | 2184×1468 | 10 |
| MIDV-047 / `midv00047` | 唯一命中 | 成功 | 2184×1468 | 12 |
| STARS-141 / `1stars00141` | 未命中 | 成功 | 800×565 | 19 |
| ABP-906 / `118abp906` | 未命中 | null | 无 | 无 |
| IPVR-231 / `ipvr00231` | 唯一命中 | 成功，VR | 797×600 | 12 |
| FUYU-079 / `fuyu079` | 紧凑格式唯一命中 | 成功，素人 | largeUrl 为 null；mediumUrl 实测 300×300 | 5 |
| GLOD-323T / `196glod0323t` | 未命中 | null | 无 | 无 |
| 人工构造的不存在 ID | 未命中 | null | 无 | 无 |

这里只能说明测试内容 ID 的当前数字视频响应；不能据此断言 ABP-906 或 GLOD-323T 在 FANZA 所有销售分类中均不存在。本轮没有实现或验证实体 DVD 的 HTML 抓取。

共 7 个历史真实商品样本：5 个已知 ID 可取详情，4 个可按本轮番号格式检索；另有 1 个负样本。该样本集不是随机片库，不能将这些比例当成全站覆盖率。

SSIS-001 的普通预览图实测为 800×534、533×800，说明官方预览原图也不等于高分辨率。FUYU-079 的封面回退成功仅代表有可用图，不代表取得了高清封面。

## 支撑独立详情展示的字段

| 详情需要 | FANZA 数据及验证结果 |
| --- | --- |
| 影片身份、标题、简介 | `id`、`makerContentId`、`title`、`description`；成功详情样本均有 |
| 分类 | `floor`；验证 AV 和 AMATEUR，VR 另有专用预告字段 |
| 日期、时长 | `deliveryStartDate`、`makerReleasedAt`、`duration`；发行与配信日期应区分，时间戳按来源时区解释，duration 单位为秒 |
| 演员及身份 | `actresses` 的 id/name/imageUrl；素人使用 `amateurActress`，并非一定有正式演员名单 |
| 厂商、品牌、系列、导演 | maker、label、series、directors 的 id/name；系列或导演可为空 |
| 标签 | genres 的 id/name；使用 FANZA 身份，不能冒充 JavDB 标签 ID |
| 评分 | reviewSummary；保留 FANZA 名称与量表，不显示成 JavDB 评分 |
| 封面、预览 | packageImage、sampleImages；下载和解码通过，但分辨率参差且允许缺失 |
| 演员其他作品 | legacySearchPPV 的 actressIds 筛选已验证：演员 `1006229` 返回总数 118、首批 8 条，逐条演员 ID 均匹配 |
| 演员头像 | 单个头像实测可下载，125×125；不足以认定高清头像覆盖通过 |
| 预告片 | 返回 MP4/HLS 或 VR 地址；SSIS-001 的 MP4 与 HLS 在直连、代理下实际 GET 均为 403，未通过可播放验收 |
| 个性化/关联推荐 | 官方脚本有推荐查询，但本轮未验证；不可与已通过的演员作品筛选混为一谈 |
| 磁力、字幕与入库状态 | 属于独立资源服务和 Miyabi 本地业务，不由 FANZA 元数据提供 |

这些数据足以证明“不依赖 JavDB 也能取得详情主体字段”的可行性。后端已接入并可将 FANZA 资料独立入库；前端仍需实现来源无关的详情展示，不能将本站未知字段补成虚假的 JavDB 信息。

## 接入决策

1. FANZA 已作为第一个影片资料与图片来源，采用已验证的 GraphQL 搜索和详情路径，不接入已受限的统一 HTML 搜索。
2. 正式适配器先检索内容 ID，再批量查询官方番号核对候选，最后只下载已匹配影片的详情，避免猜测厂商 ID 或逐条抓取大量无关影片。单个检索词最多检查 120 条候选，超限报错；同番号不同内容 ID 拒绝自动选择。请求复用现有代理、安全下载器与限流，使用来源缓存和同请求合并。
3. 无正式演员、无系列、缺导演可以保留为空；低分辨率图片和不可用预告片不能伪装成完整补全成功。
4. 未找到时交给其他通过验收的来源，最终再由 JavDB 兜底。需要借助 JavDB 才能确认身份的结果，不算“无 JavDB 独立刮削”验收通过。
5. 本轮短期测试不能证明长期稳定性，也没有完成全量片库对比；正式支持范围应限定为已验证场景。

## 正式适配器复验

- `TestFANZALive` 通过 `MIYABI_TEST_FANZA_CODES` 开启，`MIYABI_TEST_PROXY` 指定可选代理。未设置时常规测试不联网，不在仓库中保存真实图片。
- 使用 `http://127.0.0.1:10777` 对 SSIS-001、MIDV-047、IPVR-231、FUYU-079 复验，实际适配器的搜索、批量身份核对、详情获取与封面完整解码均通过，未请求 JavDB。
- 非联网测试覆盖跨页匹配、同编号多商品、错误/残缺 GraphQL 响应、素人演员与 mediumUrl 回退。metadata 层验证了 FANZA 资料齐全时跳过 JavDB、缓存复用和网络失败不降级身份。
- 图片兜底恢复测试验证：FANZA 封面失败后使用 JavDB，主来源演员与资料保留；发布事务失败后重试复用检查点，不重新查询或下载，JavDB 身份随最终影片记录一起提交。

## 参考

- MetaTube FANZA：[适配器与检索规则](https://github.com/metatube-community/metatube-sdk-go/blob/19a92ad3263ab7b69636d26d7cc32f531c0a0804/provider/fanza/fanza.go)、[历史测试商品](https://github.com/metatube-community/metatube-sdk-go/blob/19a92ad3263ab7b69636d26d7cc32f531c0a0804/provider/fanza/fanza_test.go)。
- FANZA 官方：[数字视频页面](https://video.dmm.co.jp/av/content/?id=ssis00001)、[本轮提取检索定义的公开前端文件](https://assets.video.dmm.co.jp/_next/static/chunks/494-078ebcb0c5dd5deb-pc-20261001115903-a49fdd1.js)。
