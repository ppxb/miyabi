package drive

import (
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

var (
	// ErrSourceChanged indicates the mounted media directory or authenticated 115 account has changed.
	ErrSourceChanged = domain.E(domain.KindConflict, "媒体目录或登录账号已变更，请重新扫描", nil)

	// ErrMediaDirectoryRequired indicates operations requiring a mounted directory were attempted without one.
	ErrMediaDirectoryRequired = domain.E(domain.KindInvalid, "请先在设置页挂载当前网盘账号的媒体目录", nil)

	// ErrDirectoryIncomplete indicates directory listing changed or returned partial pages during traversal.
	ErrDirectoryIncomplete = domain.E(domain.KindConflict, "网盘目录内容在读取期间变化或分页不完整，请重新扫描", nil)

	// ErrTranscodeUnavailable indicates the provider has not produced a playable stream.
	ErrTranscodeUnavailable = domain.E(domain.KindUpstream, "网盘暂未提供该文件的播放地址，请稍后重试", pan.ErrTranscodeUnavailable)
)
