## 媒体库问题清单

2. 每张卡渲染两张封面图 + blur-xl

  证据：movie-cover.tsx:11 对所有 MovieCard 开启 blurredBackground，media-image.tsx:56-61 会再渲染一个同 src 的模糊 <img>。首页 36 张卡 =
  72 个 <img>、36 个模糊图层；模糊滤镜在大尺寸元素上对 GPU 不便宜，低端设备滚动和重绘会明显吃力。

  建议：卡片场景去掉 blurredBackground（留
  bg-muted/渐变色兜底留白），只在影片详情头图保留模糊；如确实要背景图，改为容器伪元素背景，至少省掉第二个 <img> 节点。

  顺带：封面走的是全尺寸原图（imageURL → /api/image
  代理，无缩略图参数）。如果还要压首屏开销，可以给图片代理加尺寸参数、卡片只取缩略图——后端改动，优先级低于上面两条。

  3. MediaImage 的每图双保险

  media-image.tsx:21-26, 66-76：
  - key={JSON.stringify([source, original, nsfwMode])}：切换 NSFW 模式会把整页封面卸载重挂，出现占位图标闪烁；source 重置其实只依赖
    source/original。
  - onLoad 里 await image.decode() + try/catch：onLoad 已表示可显示，decode 成功后也只是再
    setStatus('ready')，多一次异步和多一次渲染。直接 setStatus('ready') 即可，onError 保留。

  4. 货架滚动的监听（低优先级）

  movie-shelf.tsx:42-59：每个 scroll 事件同步读 scrollWidth/clientWidth/scrollLeft 并 3 次 setState。React
  对相同值会跳过渲染，但布局读取仍每次发生；用 rAF 合并一下即可，收益不大。

  二、过度兜底 / 可删代码

  1. history-card.ts 整个文件是死代码：historyToCard 无任何引用（全库 grep 只有定义）；同样的映射 + 进度计算在 history/page.tsx:59-62 和
     continue-watching.tsx:15-18 各写了一遍。要么两处复用它，要么移回收站（不直接删）。
  2. 进度 clamp 做了两遍：movie-card.tsx:83 又 clamp 一次，调用方（history 页）已经 clamp 过；history-card.ts 的注释自己也写了 "the card
     also clamps defensively"。保留卡片这一处即可。
  3. 徽章外层条件与内层完全重复：movie-card.tsx:56-74 外层判断的 5 个条件，内层又逐项判断；全 false 时 Fragment
     本来也不渲染，直接去掉外层三元。
  4. 静默失败（可选）：topRated/history 请求失败时 ??
     []，对应货架直接消失、无任何提示（page.tsx:55-58、continue-watching.tsx:8）。不一定要加错误 UI，但可以留一句"该分区暂不可用"。
  5. 不建议动的：player-dialog 里那些 ref、bare store 读取、seek().catch 都有注释且对应真实竞态；task-events
     的重连/修订号也是服务端重启场景的必要逻辑。这些不是过度兜底。

  三、UI/交互小项

  1. 首页"最近加入""高分影片"没有"查看全部"：MovieShelf 已支持 href（movie-shelf.tsx:97-101），但 page.tsx:53-58 没传；/movies 支持
     sort=rating（features/movies/search.ts:51），需要让 MovieShelf 透传 search，或先都链到 /movies。
  2. 首屏无骨架：数据 pending 时货架 count=0 直接不渲染（movie-shelf.tsx:18），页面只剩标题，随后卡片整块出现；加 1-2 个货架骨架会更稳。
  3. 播放按钮只在 hover 显示（movie-card.tsx:91），键盘 Tab 聚焦时看不到；补
     group-focus-visible:opacity-100。触屏点击整卡即可播放，问题不大。
  4. 封面 <img> 没有 alt（MovieCover 不传），可传 title 改善无障碍。
  5. 卡片是 aspect-3/2 横版 + object-contain：如果封面主要是竖图，两侧留白只能靠模糊背景撑（也就是上面第 2
     点的成本来源）。要不要改竖版卡片属于产品取舍，需要你确认设计意图。
