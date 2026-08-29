/**
 * Shared formatting utilities — single source of truth.
 */

export function formatBytes(value: number | string | null | undefined): string {
  const n = Number(value || 0)
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const
  const idx = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)))
  return `${(n / Math.pow(1024, idx)).toFixed(idx === 0 ? 0 : 2)} ${units[idx]}`
}

export function formatDate(value: string | number | Date | null | undefined): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

export function short(value: string | null | undefined, length: number = 44): string {
  if (!value) return ''
  return value.length > length ? `${value.slice(0, length)}...` : value
}

export function formatLocalTime(utcString: string | null | undefined): string {
  if (!utcString) return '-'
  const d = new Date(String(utcString).endsWith('Z') ? utcString : utcString + 'Z')
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}

/**
 * 根据节点名称智能匹配所在国家/地区的国旗 Emoji
 */
export function getNodeFlag(name: string | null | undefined): string {
  if (!name) return '🌐'
  const s = String(name).toUpperCase()

  // 常见国旗直接提取
  if (/[\uD83C][\uDDE6-\uDDFF]{2}/.test(name)) {
    const m = name.match(/[\uD83C][\uDDE6-\uDDFF]{2}/)
    if (m) return m[0]
  }

  if (s.includes('香港') || s.includes('HK') || s.includes('HONG KONG') || s.includes('HKG')) return '🇭🇰'
  if (s.includes('日本') || s.includes('JP') || s.includes('JAPAN') || s.includes('东京') || s.includes('大阪') || s.includes('TYO')) return '🇯🇵'
  if (s.includes('美国') || s.includes('US') || s.includes('USA') || s.includes('UNITED STATES') || s.includes('洛杉矶') || s.includes('圣何塞') || s.includes('西雅图') || s.includes('纽约')) return '🇺🇸'
  if (s.includes('新加坡') || s.includes('SG') || s.includes('SINGAPORE') || s.includes('狮城') || s.includes('SIN')) return '🇸🇬'
  if (s.includes('台湾') || s.includes('TW') || s.includes('TAIWAN') || s.includes('台北') || s.includes('新北')) return '🇹🇼'
  if (s.includes('韩国') || s.includes('KR') || s.includes('KOREA') || s.includes('首尔') || s.includes('SEL')) return '🇰🇷'
  if (s.includes('英国') || s.includes('UK') || s.includes('GB') || s.includes('UNITED KINGDOM') || s.includes('伦敦') || s.includes('LON')) return '🇬🇧'
  if (s.includes('德国') || s.includes('DE') || s.includes('GERMANY') || s.includes('法兰克福') || s.includes('FRA')) return '🇩🇪'
  if (s.includes('法国') || s.includes('FR') || s.includes('FRANCE') || s.includes('巴黎')) return '🇫🇷'
  if (s.includes('加拿大') || s.includes('CA') || s.includes('CANADA') || s.includes('温哥华') || s.includes('多伦多')) return '🇨🇦'
  if (s.includes('澳大利亚') || s.includes('澳洲') || s.includes('AU') || s.includes('AUSTRALIA') || s.includes('悉尼')) return '🇦🇺'
  if (s.includes('俄罗斯') || s.includes('RU') || s.includes('RUSSIA') || s.includes('莫斯科')) return '🇷🇺'
  if (s.includes('荷兰') || s.includes('NL') || s.includes('NETHERLANDS') || s.includes('阿姆斯特丹')) return '🇳🇱'
  if (s.includes('印度') || s.includes('IN') || s.includes('INDIA')) return '🇮🇳'
  if (s.includes('土耳其') || s.includes('TR') || s.includes('TURKEY') || s.includes('伊斯坦布尔')) return '🇹🇷'
  if (s.includes('阿根廷') || s.includes('AR') || s.includes('ARGENTINA')) return '🇦🇷'
  if (s.includes('巴西') || s.includes('BR') || s.includes('BRAZIL')) return '🇧🇷'
  if (s.includes('马来西亚') || s.includes('MY') || s.includes('MALAYSIA')) return '🇲🇾'
  if (s.includes('泰国') || s.includes('TH') || s.includes('THAILAND')) return '🇹🇭'
  if (s.includes('菲律宾') || s.includes('PH') || s.includes('PHILIPPINES')) return '🇵🇭'
  if (s.includes('越南') || s.includes('VN') || s.includes('VIETNAM')) return '🇻🇳'
  if (s.includes('直连') || s.includes('DIRECT')) return '⚡'
  if (s.includes('自动') || s.includes('AUTO') || s.includes('FALLBACK')) return '🔄'

  return '🌐'
}
