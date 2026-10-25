export type DnsFieldType = 'list' | 'scalar-text' | 'scalar-select' | 'bool' | 'json'

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

export type DnsGroupId = 'connection-bootstrap' | 'fakeip-cache' | 'fallback-policy' | 'hosts'

export interface DnsGroupDef {
  id: DnsGroupId
  titleZh: string
  titleEn: string
  descZh: string
  descEn: string
  fields: DnsFieldDef[]
}

/**
 * Basic DNS fields displayed directly.
 */
export const DNS_BASIC_FIELDS: DnsFieldDef[] = [
  {
    key: 'nameserver',
    type: 'list',
    labelZh: '主解析服务器',
    labelEn: 'Primary Nameservers',
    descZh: '主要 DNS 上游服务器列表，支持 IP、DoH、DoT 格式，每行一个。',
    descEn: 'Primary upstream DNS servers. Supports IP, DoH, DoT, one per line.',
    placeholder: '1.1.1.1\n8.8.8.8\nhttps://dns.google/dns-query',
  },
  {
    key: 'enhanced-mode',
    type: 'scalar-select',
    labelZh: '增强模式',
    labelEn: 'Enhanced Mode',
    descZh: '选择 Fake-IP 虚拟 IP 池或 redir-host 真实 IP 解析。未指定则遵循内核缺省。',
    descEn: 'Select Fake-IP pool mode or redir-host real IP resolution. Unspecified defaults to core.',
    options: [
      { value: '', labelZh: '未指定 (遵循内核缺省)', labelEn: 'Unspecified (core default)' },
      { value: 'fake-ip', labelZh: 'fake-ip (虚拟 IP 池)', labelEn: 'fake-ip (virtual IP pool)' },
      { value: 'redir-host', labelZh: 'redir-host (真实解析)', labelEn: 'redir-host (real IP)' },
    ],
  },
  {
    key: 'ipv6',
    type: 'bool',
    labelZh: 'IPv6 解析',
    labelEn: 'Resolve IPv6',
    descZh: '是否发起 AAAA 记录查询并解析 IPv6 地址。',
    descEn: 'Whether to query AAAA records and resolve IPv6 addresses.',
  },
]

/**
 * Grouped configuration sections.
 */
export const DNS_GROUPS: DnsGroupDef[] = [
  {
    id: 'connection-bootstrap',
    titleZh: '连接与引导',
    titleEn: 'Connection & Bootstrap',
    descZh: '引导服务器、节点/直连专用解析服务器、客户端监听与连接策略',
    descEn: 'Bootstrap servers, proxy/direct dedicated nameservers, client listen address and connection policy',
    fields: [
      {
        key: 'default-nameserver',
        type: 'list',
        labelZh: '引导解析服务器',
        labelEn: 'Bootstrap Nameservers',
        descZh: '用于解析以域名形式提供的 DoH/DoT 等上游 DNS 地址，每行一个（必须为纯 IP，可为加密 DNS）。',
        descEn: 'DNS servers used to resolve domain-based DoH/DoT upstream addresses, one per line (must be IP, can be encrypted DNS).',
        placeholder: '114.114.114.114\n223.5.5.5',
      },
      {
        key: 'proxy-server-nameserver',
        type: 'list',
        labelZh: '节点域名解析服务器',
        labelEn: 'Proxy Server Nameservers',
        descZh: '用于解析代理节点服务器自身域名的专用 DNS，每行一个；不填则遵循 nameserver-policy、nameserver 与 fallback。',
        descEn: 'Dedicated DNS servers for resolving proxy node server domains, one per line; defaults to nameserver-policy, nameserver, and fallback.',
        placeholder: 'https://dns.alidns.com/dns-query',
      },
      {
        key: 'direct-nameserver',
        type: 'list',
        labelZh: '直连解析服务器',
        labelEn: 'Direct Nameservers',
        descZh: '直连出站流量使用的 DNS 解析服务器，每行一个；不填则遵循 nameserver-policy、nameserver 与 fallback。',
        descEn: 'DNS servers used for direct outbound traffic, one per line; defaults to nameserver-policy, nameserver, and fallback.',
        placeholder: '223.5.5.5\n119.29.29.29',
      },
      {
        key: 'listen',
        type: 'scalar-text',
        labelZh: '客户端监听地址',
        labelEn: 'Client Listen Address',
        descZh: '导出的客户端本地 DNS 监听 IP 与端口（如 0.0.0.0:1053，供客户端设备本地解析使用，非 CSP 控制面板服务端）。',
        descEn: 'Exported client-side local DNS listen address and port (e.g. 0.0.0.0:1053 for client device resolution, not CSP server).',
        placeholder: '0.0.0.0:1053',
      },
      {
        key: 'respect-rules',
        type: 'bool',
        labelZh: '遵循分流规则',
        labelEn: 'Respect Rules',
        descZh: 'DNS 查询连接是否遵循路由规则。注意：需配合配置 proxy-server-nameserver，且与 prefer-h3 冲突，不建议同时启用。',
        descEn: 'Whether DNS queries follow routing rules. Advisory: requires proxy-server-nameserver and conflicts with prefer-h3 (do not enable both).',
      },
      {
        key: 'direct-nameserver-follow-policy',
        type: 'bool',
        labelZh: '直连遵循 nameserver-policy',
        labelEn: 'Direct Follow Nameserver Policy',
        descZh: '直连解析服务器是否遵循 nameserver-policy（域名解析策略映射）；仅当 direct-nameserver 非空时生效。',
        descEn: 'Whether direct nameservers follow nameserver-policy (domain DNS mapping); effective only when direct-nameserver is configured.',
      },
      {
        key: 'prefer-h3',
        type: 'bool',
        labelZh: '优先 HTTP/3',
        labelEn: 'Prefer HTTP/3',
        descZh: '是否优先尝试使用 HTTP/3 (DoH3) 连接支持的上游服务器。注意：与 respect-rules 冲突，不建议同时启用。',
        descEn: 'Whether to prefer HTTP/3 (DoH3) when querying supported upstream servers. Advisory: conflicts with respect-rules.',
      },
    ],
  },
  {
    id: 'fakeip-cache',
    titleZh: 'Fake-IP 与缓存',
    titleEn: 'Fake-IP & Cache',
    descZh: '虚拟 IP 池 CIDR 范围、域名过滤列表及内存缓存算法',
    descEn: 'Virtual IP CIDR range, domain filter list, and memory cache eviction algorithm',
    fields: [
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
        descZh: 'Fake-IP 域名通配与过滤列表，每行一条。默认黑名单模式下匹配域名不分配 Fake-IP；若在 Raw 中配置了自定义 fake-ip-filter-mode（如 whitelist 或 rule），则遵循相应模式规则。',
        descEn: 'Fake-IP domain filter patterns, one per line. In default blacklist mode, matched domains bypass Fake-IP; follows custom fake-ip-filter-mode (whitelist/rule) if configured.',
        placeholder: '*.lan\n+.local\nlocalhost.ptlogin2.qq.com',
      },
      {
        key: 'cache-algorithm',
        type: 'scalar-select',
        labelZh: '缓存淘汰算法',
        labelEn: 'Cache Algorithm',
        descZh: 'DNS 内部内存缓存淘汰算法，未指定则遵循内核缺省。',
        descEn: 'DNS cache eviction algorithm, unspecified defaults to core policy.',
        options: [
          { value: '', labelZh: '默认 (未指定)', labelEn: 'Default (unspecified)' },
          { value: 'arc', labelZh: 'arc (自适应替换缓存)', labelEn: 'arc (Adaptive Replacement Cache)' },
          { value: 'bip', labelZh: 'bip (双队列缓存)', labelEn: 'bip (Bimodal Insertion Policy)' },
        ],
      },
    ],
  },
  {
    id: 'fallback-policy',
    titleZh: '回退与域名策略',
    titleEn: 'Fallback & Domain Policy',
    descZh: '后备解析服务器、按域名/分类指定上游策略及后备过滤条件',
    descEn: 'Fallback nameservers, domain policy mappings, and fallback filter criteria',
    fields: [
      {
        key: 'fallback',
        type: 'list',
        labelZh: '后备解析服务器',
        labelEn: 'Fallback Nameservers',
        descZh: '后备 DNS 上游列表。默认配合 fallback-filter 筛选结果，当主 DNS 解析结果满足污染条件（如非 CN 地区 IP 或匹配网段）时采用后备 DNS 结果。',
        descEn: 'Fallback DNS upstream servers. Works with fallback-filter: fallback results are selected when primary DNS responses meet pollution criteria (e.g. non-CN IP or matched CIDR).',
        placeholder: '8.8.8.8\n1.0.0.1\nhttps://cloudflare-dns.com/dns-query',
      },
      {
        key: 'nameserver-policy',
        type: 'json',
        labelZh: '域名解析策略',
        labelEn: 'Nameserver Policy',
        descZh: '指定域名、geosite 或 rule-set 使用专属上游解析器 (标准 JSON 对象)，优先于 nameserver/fallback。',
        descEn: 'Route specific domains, geosites or rule-sets to dedicated DNS (valid JSON object), takes precedence over nameserver/fallback.',
        example: '{\n  "geosite:cn": "223.5.5.5",\n  "geosite:geolocation-!cn": "8.8.8.8"\n}',
      },
      {
        key: 'fallback-filter',
        type: 'json',
        labelZh: '后备过滤条件',
        labelEn: 'Fallback Filter',
        descZh: '判定主 DNS 结果是否被污染并采用 fallback 结果的筛选条件（标准 JSON 对象，支持 geoip、geoip-code、ipcidr 等）。',
        descEn: 'Criteria determining whether primary DNS response is polluted to adopt fallback results (valid JSON object supporting geoip, geoip-code, ipcidr, etc.).',
        example: '{\n  "geoip": true,\n  "geoip-code": "CN",\n  "ipcidr": [\n    "240.0.0.0/4"\n  ]\n}',
      },
      {
        key: 'proxy-server-nameserver-policy',
        type: 'json',
        labelZh: '节点域名解析策略',
        labelEn: 'Proxy Server Nameserver Policy',
        descZh: '为不同代理节点域名指定专属解析上游 (标准 JSON 对象)；仅当 proxy-server-nameserver 非空时生效。',
        descEn: 'Route proxy node domain queries to dedicated DNS (valid JSON object); effective only when proxy-server-nameserver is configured.',
        example: '{\n  "geosite:netflix": "1.0.0.1"\n}',
      },
    ],
  },
  {
    id: 'hosts',
    titleZh: 'Hosts 主机映射',
    titleEn: 'Hosts Settings',
    descZh: '客户端 hosts 查询与系统 hosts 文件引用开关（注：顶层 hosts 映射在客户端配置中维护，不在本页编辑）',
    descEn: 'Client hosts query and system hosts switches (note: top-level static hosts are defined in client config and not edited on this page)',
    fields: [
      {
        key: 'use-hosts',
        type: 'bool',
        labelZh: '使用客户端配置 Hosts',
        labelEn: 'Use Client Config Hosts',
        descZh: '客户端解析时是否匹配导出的客户端配置中定义的 hosts 规则（注：顶层静态 hosts 映射由客户端配置决定，不在本页编辑）。',
        descEn: 'Whether client resolution queries static hosts rules defined in the exported client config (top-level hosts are not edited on this page).',
      },
      {
        key: 'use-system-hosts',
        type: 'bool',
        labelZh: '使用客户端系统 Hosts',
        labelEn: 'Use Client System Hosts',
        descZh: '客户端解析时是否读取运行客户端设备的操作系统 hosts 文件（如客户端系统的 /etc/hosts，非 CSP 服务端）。',
        descEn: 'Whether client resolution queries the client device operating system hosts file (not CSP server hosts).',
      },
    ],
  },
]

export const ALL_KNOWN_KEYS = [
  'enable',
  'nameserver',
  'enhanced-mode',
  'ipv6',
  'default-nameserver',
  'proxy-server-nameserver',
  'direct-nameserver',
  'listen',
  'respect-rules',
  'direct-nameserver-follow-policy',
  'prefer-h3',
  'fake-ip-range',
  'fake-ip-filter',
  'cache-algorithm',
  'fallback',
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
  if (type === 'json') {
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
