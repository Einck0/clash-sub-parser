/**
 * Rule presets/templates for quick insertion.
 * Each preset is a group of rules that can be applied at once.
 */

export const rulePresetCategories = [
  {
    id: 'ai',
    name: 'AI 服务',
    icon: '🤖',
    presets: [
      {
        id: 'openai',
        name: 'OpenAI / ChatGPT',
        description: 'OpenAI 全系列服务走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'openai.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'ai.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'chatgpt.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'oaiusercontent.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'openaiapi-site.azureedge.net', proxy: 'PROXY' },
          { type: 'DOMAIN-KEYWORD', value: 'openai', proxy: 'PROXY' },
        ],
      },
      {
        id: 'claude',
        name: 'Claude / Anthropic',
        description: 'Anthropic Claude 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'anthropic.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'claude.ai', proxy: 'PROXY' },
          { type: 'DOMAIN-KEYWORD', value: 'anthropic', proxy: 'PROXY' },
        ],
      },
      {
        id: 'google-ai',
        name: 'Google AI / Gemini',
        description: 'Google AI 服务走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'gemini.google.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'bard.google.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'generativelanguage.googleapis.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'ai.google.dev', proxy: 'PROXY' },
        ],
      },
      {
        id: 'copilot',
        name: 'Microsoft Copilot',
        description: 'Microsoft Copilot 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'copilot.microsoft.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'bing.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'edgeservices.bing.com', proxy: 'PROXY' },
        ],
      },
    ],
  },
  {
    id: 'media',
    name: '流媒体',
    icon: '🎬',
    presets: [
      {
        id: 'youtube',
        name: 'YouTube',
        description: 'YouTube 全系列走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'youtube.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'youtu.be', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'ytimg.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'ggpht.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'googlevideo.com', proxy: 'PROXY' },
          { type: 'DOMAIN-KEYWORD', value: 'youtube', proxy: 'PROXY' },
        ],
      },
      {
        id: 'netflix',
        name: 'Netflix',
        description: 'Netflix 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'netflix.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'netflix.net', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'nflximg.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'nflxvideo.net', proxy: 'PROXY' },
          { type: 'DOMAIN-KEYWORD', value: 'netflix', proxy: 'PROXY' },
        ],
      },
      {
        id: 'spotify',
        name: 'Spotify',
        description: 'Spotify 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'spotify.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'scdn.co', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'spotifycdn.com', proxy: 'PROXY' },
        ],
      },
      {
        id: 'bilibili',
        name: 'Bilibili',
        description: 'Bilibili 直连',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'bilibili.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'hdslb.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'biliapi.net', proxy: 'DIRECT' },
          { type: 'DOMAIN-KEYWORD', value: 'bilibili', proxy: 'DIRECT' },
        ],
      },
    ],
  },
  {
    id: 'social',
    name: '社交',
    icon: '💬',
    presets: [
      {
        id: 'twitter',
        name: 'X / Twitter',
        description: 'X (Twitter) 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'x.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'twitter.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'twimg.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 't.co', proxy: 'PROXY' },
          { type: 'DOMAIN-KEYWORD', value: 'twitter', proxy: 'PROXY' },
        ],
      },
      {
        id: 'telegram',
        name: 'Telegram',
        description: 'Telegram 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'telegram.org', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 't.me', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'telegram.me', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'telegra.ph', proxy: 'PROXY' },
          { type: 'IP-CIDR', value: '91.108.0.0/16', proxy: 'PROXY' },
          { type: 'IP-CIDR', value: '149.154.0.0/16', proxy: 'PROXY' },
        ],
      },
      {
        id: 'discord',
        name: 'Discord',
        description: 'Discord 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'discord.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'discord.gg', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'discordapp.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'discordapp.net', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'discord.media', proxy: 'PROXY' },
        ],
      },
      {
        id: 'whatsapp',
        name: 'WhatsApp',
        description: 'WhatsApp 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'whatsapp.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'whatsapp.net', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'wa.me', proxy: 'PROXY' },
          { type: 'DOMAIN-KEYWORD', value: 'whatsapp', proxy: 'PROXY' },
        ],
      },
    ],
  },
  {
    id: 'dev',
    name: '开发工具',
    icon: '💻',
    presets: [
      {
        id: 'github',
        name: 'GitHub',
        description: 'GitHub 全系列走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'github.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'github.io', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'githubusercontent.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'githubassets.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'github.dev', proxy: 'PROXY' },
        ],
      },
      {
        id: 'google-dev',
        name: 'Google 开发者',
        description: 'Google 开发者服务走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'googleapis.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'google.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'gstatic.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'googlesyndication.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'googleadservices.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'google.cn', proxy: 'DIRECT' },
        ],
      },
      {
        id: 'stackoverflow',
        name: 'Stack Overflow',
        description: 'Stack Overflow 走代理',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'stackoverflow.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'stackexchange.com', proxy: 'PROXY' },
          { type: 'DOMAIN-SUFFIX', value: 'sstatic.net', proxy: 'PROXY' },
        ],
      },
    ],
  },
  {
    id: 'cn',
    name: '国内直连',
    icon: '🇨🇳',
    presets: [
      {
        id: 'cn-direct',
        name: '国内常见服务直连',
        description: '国内常用网站直连规则',
        rules: [
          { type: 'GEOSITE', value: 'cn', proxy: 'DIRECT' },
          { type: 'GEOIP', value: 'cn', proxy: 'DIRECT' },
        ],
      },
      {
        id: 'cn-banks',
        name: '国内银行/金融',
        description: '银行和金融机构直连',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'icbc.com.cn', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'ccb.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'boc.cn', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'abchina.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'cmbchina.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'alipay.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'tenpay.com', proxy: 'DIRECT' },
        ],
      },
      {
        id: 'cn-shopping',
        name: '国内购物',
        description: '电商平台直连',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'taobao.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'tmall.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'jd.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'pinduoduo.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'douyin.com', proxy: 'DIRECT' },
          { type: 'DOMAIN-SUFFIX', value: 'meituan.com', proxy: 'DIRECT' },
        ],
      },
    ],
  },
  {
    id: 'privacy',
    name: '隐私/广告',
    icon: '🛡️',
    presets: [
      {
        id: 'adblock',
        name: '常见广告域名',
        description: '常见广告和追踪域名拦截',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'doubleclick.net', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'googlesyndication.com', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'googleadservices.com', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'facebook.com', proxy: 'REJECT' },
          { type: 'DOMAIN-KEYWORD', value: 'adservice', proxy: 'REJECT' },
          { type: 'DOMAIN-KEYWORD', value: 'analytics', proxy: 'REJECT' },
        ],
      },
      {
        id: 'privacy-trackers',
        name: '隐私追踪',
        description: '拦截常见追踪和数据收集',
        rules: [
          { type: 'DOMAIN-SUFFIX', value: 'amplitude.com', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'mixpanel.com', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'segment.com', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'hotjar.com', proxy: 'REJECT' },
          { type: 'DOMAIN-SUFFIX', value: 'sentry.io', proxy: 'REJECT' },
        ],
      },
    ],
  },
]

/**
 * Get all presets flat (for search)
 */
export function getAllPresets() {
  const all = []
  for (const cat of rulePresetCategories) {
    for (const preset of cat.presets) {
      all.push({ ...preset, category: cat.name, categoryIcon: cat.icon })
    }
  }
  return all
}
