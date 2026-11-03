export type DnsFieldType = 'list' | 'scalar-text' | 'scalar-select' | 'bool' | 'yaml-mapping'

export interface DnsFieldOption {
  value: string
  labelZh: string
  labelEn: string
}

export interface DnsFieldDef {
  key: string
  type: DnsFieldType
  labelZh: string
  labelEn: string
  descZh: string
  descEn: string
  placeholder?: string
  example?: string
  options?: DnsFieldOption[]
}

export type DnsGroupId = 'routing-policy' | 'dedicated-servers' | 'switches-hosts'

export interface DnsGroupDef {
  id: DnsGroupId
  titleZh: string
  titleEn: string
  descZh: string
  descEn: string
  fields: DnsFieldDef[]
}

/**
 * 常用前置 DNS 设置（主备引导解析、增强模式、Fake-IP 池及过滤、客户端监听、IPv6 开关等直接可见，不默认折叠）。
 */
export const DNS_COMMON_FIELDS: DnsFieldDef[] = [
  {
    key: 'nameserver',
    type: 'list',
    labelZh: '主解析服务器',
    labelEn: 'Primary Nameservers',
    descZh: '主要 DNS 上游服务器列表，支持 IP、DoH、DoT 格式，支持逐项增删与多行粘贴。',
    descEn: 'Primary upstream DNS servers. Supports IP, DoH, DoT; supports item-by-item edit and multiline paste.',
    placeholder: '1.1.1.1\n8.8.8.8\nhttps://dns.google/dns-query',
  },
  {
    key: 'fallback',
    type: 'list',
    labelZh: '后备解析服务器',
    labelEn: 'Fallback Nameservers',
    descZh: '后备 DNS 上游列表。当主 DNS 结果满足污染条件时采用后备 DNS 结果。',
    descEn: 'Fallback DNS servers used when primary responses meet pollution criteria.',
    placeholder: '8.8.8.8\n1.0.0.1\nhttps://cloudflare-dns.com/dns-query',
  },
  {
    key: 'default-nameserver',
    type: 'list',
    labelZh: '引导解析服务器',
    labelEn: 'Bootstrap Nameservers',
    descZh: '用于解析 DoH/DoT 等域名形式上游 DNS 地址的服务器（必须为纯 IP）。',
    descEn: 'Bootstrap DNS servers to resolve domain-based DoH/DoT upstream addresses (must be IPs).',
    placeholder: '114.114.114.114\n223.5.5.5',
  },
  {
    key: 'enhanced-mode',
    type: 'scalar-select',
    labelZh: '增强模式',
    labelEn: 'Enhanced Mode',
    descZh: '选择 Fake-IP 虚拟 IP 池或 redir-host 真实 IP 解析。未配置则遵循内核缺省。',
    descEn: 'Select Fake-IP pool mode or redir-host real IP resolution. Unspecified defaults to core.',
    options: [
      { value: '', labelZh: '未配置 (遵循内核缺省)', labelEn: 'Unspecified (core default)' },
      { value: 'fake-ip', labelZh: 'fake-ip (虚拟 IP 池)', labelEn: 'fake-ip (virtual IP pool)' },
      { value: 'redir-host', labelZh: 'redir-host (真实解析)', labelEn: 'redir-host (real IP)' },
    ],
  },
  {
    key: 'fake-ip-range',
    type: 'scalar-text',
    labelZh: 'Fake-IP 地址池',
    labelEn: 'Fake-IP Range',
    descZh: '分配给域名的虚拟 IP CIDR 范围，如 198.18.0.1/16。',
    descEn: 'Virtual IP pool CIDR assigned to domains, e.g. 198.18.0.1/16.',
    placeholder: '198.18.0.1/16',
  },
  {
    key: 'fake-ip-filter',
    type: 'list',
    labelZh: 'Fake-IP 过滤列表',
    labelEn: 'Fake-IP Filter',
    descZh: 'Fake-IP 域名通配与过滤列表，匹配域名不分配 Fake-IP；支持逐项增删与多行粘贴。',
    descEn: 'Fake-IP domain filter patterns bypassing Fake-IP allocation.',
    placeholder: '*.lan\n+.local\nlocalhost.ptlogin2.qq.com',
  },
  {
    key: 'ipv6',
    type: 'bool',
    labelZh: 'IPv6 解析',
    labelEn: 'Resolve IPv6',
    descZh: '是否发起 AAAA 记录查询并解析 IPv6 地址。',
    descEn: 'Whether to query AAAA records and resolve IPv6 addresses.',
  },
  {
    key: 'listen',
    type: 'scalar-text',
    labelZh: '客户端监听地址',
    labelEn: 'Client Listen Address',
    descZh: '导出的客户端本地 DNS 监听 IP 与端口（如 0.0.0.0:1053，供客户端设备本地解析使用）。',
    descEn: 'Exported client-side local DNS listen address and port (e.g. 0.0.0.0:1053).',
    placeholder: '0.0.0.0:1053',
  },
]

export const DNS_BASIC_FIELDS = DNS_COMMON_FIELDS

/**
 * 高级 DNS 配置分组（高级 YAML 策略映射、专用解析上游、分流与 Hosts 行为开关）。
 */
export const DNS_ADVANCED_GROUPS: DnsGroupDef[] = [
  {
    id: 'routing-policy',
    titleZh: '高级解析策略映射 (YAML)',
    titleEn: 'Advanced Routing Policies (YAML)',
    descZh: '按域名/geosite指定专属上游、后备结果筛选条件及代理节点域名解析策略（自然 YAML 映射）',
    descEn: 'Domain-specific nameservers, fallback filters, and proxy server nameserver policies (natural YAML mappings)',
    fields: [
      {
        key: 'nameserver-policy',
        type: 'yaml-mapping',
        labelZh: '域名解析策略',
        labelEn: 'Nameserver Policy',
        descZh: '为指定域名、geosite 或 rule-set 分流指定专属上游解析器 (自然 YAML 映射)。',
        descEn: 'Route specific domains, geosites, or rule-sets to dedicated DNS (natural YAML mapping).',
        example: 'geosite:cn: 223.5.5.5\ngeosite:geolocation-!cn:\n  - 8.8.8.8\n  - 1.1.1.1',
      },
      {
        key: 'fallback-filter',
        type: 'yaml-mapping',
        labelZh: '后备过滤条件',
        labelEn: 'Fallback Filter',
        descZh: '判定主 DNS 结果是否被污染并改用 fallback 结果的过滤条件（支持 geoip, ipcidr 等）。',
        descEn: 'Criteria determining whether primary DNS response is polluted to adopt fallback results.',
        example: 'geoip: true\ngeoip-code: CN\nipcidr:\n  - 240.0.0.0/4',
      },
      {
        key: 'proxy-server-nameserver-policy',
        type: 'yaml-mapping',
        labelZh: '节点域名解析策略',
        labelEn: 'Proxy Server Nameserver Policy',
        descZh: '为不同代理节点域名指定专属解析上游；仅当 proxy-server-nameserver 配置时生效。',
        descEn: 'Route proxy node domain queries to dedicated DNS; effective when proxy-server-nameserver is configured.',
        example: 'geosite:netflix: 1.0.0.1\n"+.lan": 114.114.114.114',
      },
    ],
  },
  {
    id: 'dedicated-servers',
    titleZh: '专用解析服务器',
    titleEn: 'Dedicated Nameservers',
    descZh: '代理节点自身域名解析专用服务器与直连出站专用解析服务器',
    descEn: 'Dedicated DNS servers for proxy node domains and direct outbound traffic',
    fields: [
      {
        key: 'proxy-server-nameserver',
        type: 'list',
        labelZh: '节点域名解析服务器',
        labelEn: 'Proxy Server Nameservers',
        descZh: '用于解析代理节点服务器自身域名的专用 DNS，每行一个；支持逐项增删与多行粘贴。',
        descEn: 'Dedicated DNS servers for resolving proxy node server domains.',
        placeholder: 'https://dns.alidns.com/dns-query',
      },
      {
        key: 'direct-nameserver',
        type: 'list',
        labelZh: '直连解析服务器',
        labelEn: 'Direct Nameservers',
        descZh: '直连出站流量使用的 DNS 解析服务器，每行一个；支持逐项增删与多行粘贴。',
        descEn: 'DNS servers used for direct outbound traffic.',
        placeholder: '223.5.5.5\n119.29.29.29',
      },
    ],
  },
  {
    id: 'switches-hosts',
    titleZh: '行为、缓存与 Hosts 选项',
    titleEn: 'Behavior, Cache & Hosts',
    descZh: '分流规则遵循、HTTP/3 优先、直连策略、内存缓存算法及系统 Hosts 引用开关',
    descEn: 'Routing rule compliance, HTTP/3 preference, direct policy, cache eviction, and hosts switches',
    fields: [
      {
        key: 'cache-algorithm',
        type: 'scalar-select',
        labelZh: '缓存淘汰算法',
        labelEn: 'Cache Eviction Algorithm',
        descZh: 'DNS 内部内存缓存淘汰算法，未配置则遵循内核缺省。',
        descEn: 'DNS cache eviction algorithm, unspecified defaults to core policy.',
        options: [
          { value: '', labelZh: '未配置 (遵循内核缺省)', labelEn: 'Unspecified (core default)' },
          { value: 'arc', labelZh: 'arc (自适应替换缓存)', labelEn: 'arc (Adaptive Replacement Cache)' },
          { value: 'bip', labelZh: 'bip (双队列缓存)', labelEn: 'bip (Bimodal Insertion Policy)' },
        ],
      },
      {
        key: 'respect-rules',
        type: 'bool',
        labelZh: '遵循分流规则',
        labelEn: 'Respect Rules',
        descZh: 'DNS 查询连接是否遵循路由规则。提示：需配置 proxy-server-nameserver，且与 prefer-h3 冲突。',
        descEn: 'Whether DNS queries follow routing rules. Advisory: requires proxy-server-nameserver; conflicts with prefer-h3.',
      },
      {
        key: 'direct-nameserver-follow-policy',
        type: 'bool',
        labelZh: '直连遵循 nameserver-policy',
        labelEn: 'Direct Follow Nameserver Policy',
        descZh: '直连解析服务器是否遵循 nameserver-policy；仅当 direct-nameserver 非空时生效。',
        descEn: 'Whether direct nameservers follow nameserver-policy.',
      },
      {
        key: 'prefer-h3',
        type: 'bool',
        labelZh: '优先 HTTP/3',
        labelEn: 'Prefer HTTP/3',
        descZh: '是否优先尝试使用 HTTP/3 (DoH3) 连接支持的上游服务器。提示：与 respect-rules 冲突。',
        descEn: 'Whether to prefer HTTP/3 (DoH3) when querying upstream servers. Conflicts with respect-rules.',
      },
      {
        key: 'use-hosts',
        type: 'bool',
        labelZh: '使用客户端配置 Hosts',
        labelEn: 'Use Client Config Hosts',
        descZh: '客户端解析时是否匹配导出的客户端配置中定义的 hosts 规则。',
        descEn: 'Whether client resolution queries static hosts rules defined in the exported client config.',
      },
      {
        key: 'use-system-hosts',
        type: 'bool',
        labelZh: '使用客户端系统 Hosts',
        labelEn: 'Use Client System Hosts',
        descZh: '客户端解析时是否读取客户端设备操作系统的 hosts 文件（如客户端系统的 /etc/hosts）。',
        descEn: 'Whether client resolution queries the client device operating system hosts file.',
      },
    ],
  },
]

export const DNS_GROUPS = DNS_ADVANCED_GROUPS

export const ALL_KNOWN_KEYS = [
  'enable',
  'nameserver',
  'fallback',
  'default-nameserver',
  'enhanced-mode',
  'fake-ip-range',
  'fake-ip-filter',
  'ipv6',
  'listen',
  'proxy-server-nameserver',
  'direct-nameserver',
  'respect-rules',
  'direct-nameserver-follow-policy',
  'prefer-h3',
  'cache-algorithm',
  'nameserver-policy',
  'fallback-filter',
  'proxy-server-nameserver-policy',
  'use-hosts',
  'use-system-hosts',
] as const

const KNOWN_KEY_SET = new Set<string>(ALL_KNOWN_KEYS)

export function getUnknownKeys(draft: Record<string, unknown>): string[] {
  return Object.keys(draft).filter((k) => !KNOWN_KEY_SET.has(k))
}

export function getBoolDisplayState(
  draft: Record<string, unknown>,
  key: string
): 'unspecified' | 'true' | 'false' | 'custom' {
  if (!(key in draft) || draft[key] === undefined || draft[key] === null || draft[key] === '') {
    return 'unspecified'
  }
  const val = draft[key]
  if (val === true) return 'true'
  if (val === false) return 'false'
  return 'custom'
}

export function isFieldConfigured(
  draft: Record<string, unknown>,
  key: string,
  type: DnsFieldType
): boolean {
  if (!(key in draft) || draft[key] === undefined || draft[key] === null) {
    return false
  }
  const val = draft[key]
  if (type === 'list') {
    return Array.isArray(val) && val.length > 0
  }
  if (type === 'scalar-text') {
    return typeof val === 'string' && val.trim().length > 0
  }
  if (type === 'scalar-select') {
    return typeof val === 'string' && val.length > 0
  }
  if (type === 'bool') {
    return typeof val === 'boolean'
  }
  if (type === 'yaml-mapping') {
    return (
      typeof val === 'object' &&
      val !== null &&
      !Array.isArray(val) &&
      Object.keys(val as object).length > 0
    )
  }
  return true
}

export function countGroupConfigured(
  draft: Record<string, unknown>,
  group: DnsGroupDef
): number {
  let count = 0
  for (const field of group.fields) {
    if (isFieldConfigured(draft, field.key, field.type)) {
      count++
    }
  }
  return count
}
