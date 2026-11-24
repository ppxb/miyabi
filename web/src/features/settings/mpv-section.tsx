import { DownloadIcon, PlayIcon, Trash2Icon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { SettingRow, SettingsSection } from './shared'

function downloadScript(path: string, filename: string) {
  const link = document.createElement('a')
  link.href = path
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
}

/**
 * Outputs target for the local MPV player. The browser cannot launch a local
 * executable, so the user installs a small per-user URL protocol bridge once;
 * afterwards "Play with MPV" on a movie card can start the local mpv together
 * with its subtitle.
 */
export function MpvSection() {
  return (
    <SettingsSection icon={<PlayIcon className="size-4" />} title="MPV 播放器">
      <SettingRow
        title="使用 MPV 播放"
        description="安装脚本后可直接唤起本机 MPV 播放。首次使用请下载安装脚本并执行。"
      >
        <div className="flex items-center gap-2">
          <Button
            type="button"
            onClick={() => downloadScript('/api/mpv-bridge/install.ps1', 'install-mpv-bridge.ps1')}
          >
            <DownloadIcon data-icon="inline-start" />
            下载安装脚本
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              downloadScript('/api/mpv-bridge/uninstall.ps1', 'uninstall-mpv-bridge.ps1')
            }
          >
            <Trash2Icon data-icon="inline-start" />
            下载卸载脚本
          </Button>
        </div>
      </SettingRow>
    </SettingsSection>
  )
}
