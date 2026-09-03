<template>
  <section class="page nodes-page">
    <!-- Top Header -->
    <div class="page-head">
      <div>
        <p class="eyebrow">Node Quality Control & Routing Ledger</p>
        <h2>节点管理与质检中心</h2>
        <p class="page-desc">
          查看所有订阅与手动节点，进行真实出站握手测速、流媒体与 AI 解锁全项质检，以及配置跳板代理链路。
        </p>
      </div>
      <div class="action-row">
        <button
          class="primary btn-glow"
          @click="startProbeBatch(filteredOrSelectedNodes)"
          :disabled="probing || !rows.length"
          title="对当前节点列表执行全协议真实探测"
        >
          <span v-if="probing" class="spinner-inline"></span>
          {{ probing ? `质检中 (${probeProgress.done}/${probeProgress.total})…` : '⚡ 综合质检' }}
        </button>
        <button
          v-if="probing"
          class="danger"
          @click="cancelProbeBatch"
          title="中断正在进行的探测"
        >
          停止探测
        </button>
        <button @click="reload" :disabled="loading">
          🔄 刷新数据
        </button>
      </div>
    </div>

    <!-- Alert / Error message -->
    <div v-if="error" class="alert error">{{ error }}</div>

    <!-- Live Probe Progress Banner -->
    <div v-if="probing" class="probe-progress-card">
      <div class="progress-info">
        <div class="progress-title">
          <span class="spinner-inline"></span>
          <strong>全协议深度质检进行中</strong>
          <span class="progress-nums">{{ probeProgress.done }} / {{ probeProgress.total }} ({{ probeProgressPercent }}%)</span>
        </div>
        <div class="progress-stats">
          <span class="stat-tag stat-ok">🟢 正常: {{ probeProgress.ok }}</span>
          <span class="stat-tag stat-fail">🔴 失败: {{ probeProgress.fail }}</span>
          <span v-if="includeSpeedtest" class="stat-tag stat-speed">🚀 测速中</span>
        </div>
      </div>
      <div class="progress-bar-bg">
        <div class="progress-bar-fill" :style="{ width: `${probeProgressPercent}%` }"></div>
      </div>
    </div>

    <!-- Top Insights & Stats Dashboard -->
    <div class="insights-grid">
      <!-- Total Nodes & Filtered -->
      <div class="metric-card" @click="resetFilters" role="button" title="点击重置全部筛选">
        <div class="metric-head">
          <span class="metric-label">节点总览</span>
          <span class="metric-icon">🌐</span>
        </div>
        <div class="metric-body">
          <strong class="metric-val">{{ rows.length }}</strong>
          <span class="metric-sub" v-if="filtered.length !== rows.length">匹配 {{ filtered.length }} 个</span>
          <span class="metric-sub" v-else>涵盖 {{ subOptions.length }} 个订阅 · {{ typeOptions.length }} 种协议</span>
        </div>
      </div>

      <!-- Health Rate & Latency -->
      <div class="metric-card" :class="{ active: statusFilter === 'ok' }" @click="toggleStatusFilter('ok')" role="button" title="点击仅看正常节点">
        <div class="metric-head">
          <span class="metric-label">在线健康率</span>
          <span class="metric-icon">🟢</span>
        </div>
        <div class="metric-body">
          <strong class="metric-val" :class="healthRateClass">{{ testedCount ? `${healthPercent}%` : '未测试' }}</strong>
          <span class="metric-sub">
            {{ healthyCount }}/{{ testedCount || rows.length }} 正常 · 平均时延 {{ avgLatency ? `${avgLatency}ms` : '-' }}
          </span>
        </div>
      </div>

      <!-- High Speed Gauge -->
      <div class="metric-card" :class="{ active: speedFilter > 0 }" @click="toggleSpeedFilter(10)" role="button" title="点击仅看测速 >= 10Mbps 节点">
        <div class="metric-head">
          <span class="metric-label">高速带宽节点</span>
          <span class="metric-icon">🚀</span>
        </div>
        <div class="metric-body">
          <strong class="metric-val">{{ highSpeedCount }}</strong>
          <span class="metric-sub">
            {{ testedSpeedCount }} 个已测速 · 最高 {{ maxSpeed ? `${maxSpeed}M` : '-' }}
          </span>
        </div>
      </div>

      <!-- Chained Proxy Count -->
      <div class="metric-card" :class="{ active: chainFilter === 'chained' }" @click="toggleChainFilter" role="button" title="点击仅看已挂跳板节点">
        <div class="metric-head">
          <span class="metric-label">已挂链节点</span>
          <span class="metric-icon">🔗</span>
        </div>
        <div class="metric-body">
          <strong class="metric-val">{{ chainedCount }}</strong>
          <span class="metric-sub">{{ rows.length - chainedCount }} 个直连节点</span>
        </div>
      </div>
    </div>

    <!-- AI & Streaming Unlock Overview Matrix -->
    <div class="media-matrix-card">
      <div class="matrix-header">
        <span class="matrix-title">🎬 流媒体与 AI 解锁概览</span>
        <span class="matrix-hint">点击对应徽章可快捷筛选解锁节点</span>
      </div>
      <div class="matrix-badges">
        <div
          v-for="platform in mediaPlatformList"
          :key="platform.key"
          class="matrix-chip"
          :class="{
            active: selectedMediaFilters.includes(platform.key),
            has_unlocked: (mediaStats[platform.key] || 0) > 0,
          }"
          @click="toggleMediaFilter(platform.key)"
          :title="`点击筛选支持 ${platform.name} 的节点`"
        >
          <span class="chip-icon">{{ platform.icon }}</span>
          <span class="chip-name">{{ platform.name }}</span>
          <strong class="chip-count">{{ mediaStats[platform.key] || 0 }}</strong>
        </div>
      </div>
    </div>

    <!-- Advanced Multi-Dimensional Filter Toolbar -->
    <div class="control-panel-card">
      <!-- Search & Main Dropdowns -->
      <div class="filter-row main-filters">
        <!-- Search -->
        <div class="search-input-wrap">
          <span class="search-icon">🔍</span>
          <input
            v-model.trim="search"
            type="search"
            placeholder="搜索节点名 / 出口 IP / 服务器 / 端口 / 协议 / 跳板 / 策略组…"
            class="ledger-search-input"
          />
          <button v-if="search" class="clear-btn" @click="search = ''" title="清空搜索">✕</button>
        </div>

        <!-- Subscription Dropdown -->
        <select v-model="subFilter" class="filter-select" title="按订阅来源筛选">
          <option value="">📁 全部订阅 ({{ rows.length }})</option>
          <option v-for="s in subOptions" :key="s" :value="s">{{ s }}</option>
        </select>

        <!-- Protocol Dropdown -->
        <select v-model="typeFilter" class="filter-select" title="按节点协议筛选">
          <option value="">⚡ 全部协议</option>
          <option v-for="t in typeOptions" :key="t" :value="t">{{ t.toUpperCase() }}</option>
        </select>

        <!-- Health Status Filter -->
        <select v-model="statusFilter" class="filter-select" title="按健康与探测状态筛选">
          <option value="all">🚦 全部状态</option>
          <option value="ok">🟢 仅看正常可用</option>
          <option value="fast">⚡ 极速低延迟 (&lt;300ms)</option>
          <option value="medium">🟡 普通延迟 (300-800ms)</option>
          <option value="fail">🔴 离线或探测失败</option>
          <option value="untested">⚪ 尚未探测</option>
        </select>

        <!-- Dialer Chain Filter -->
        <select v-model="chainFilter" class="filter-select" title="按链路挂载筛选">
          <option value="all">🔗 全部链路</option>
          <option value="chained">仅看已挂链</option>
          <option value="plain">仅看未挂链 (直连)</option>
        </select>

        <!-- Sort Select -->
        <select v-model="sortBy" class="filter-select sort-select" title="排序规则">
          <option value="default">默认排列</option>
          <option value="latency_asc">⚡ 延迟从低到高</option>
          <option value="latency_desc">⚡ 延迟从高到低</option>
          <option value="speed_desc">🚀 测速从高到低</option>
          <option value="name_asc">🏷️ 节点名 (A-Z)</option>
          <option value="country">🌍 国家地区</option>
          <option value="checked_desc">⏱️ 最近质检时间</option>
        </select>
      </div>

      <!-- Quick Country / Region Pills -->
      <div v-if="countryOptions.length" class="country-pills-row">
        <span class="pills-label">快捷地区:</span>
        <button
          class="country-pill"
          :class="{ active: countryFilter === '' }"
          @click="countryFilter = ''"
        >
          全部 ({{ rows.length }})
        </button>
        <button
          v-for="c in countryOptions"
          :key="c.code"
          class="country-pill"
          :class="{ active: countryFilter === c.code }"
          @click="countryFilter = countryFilter === c.code ? '' : c.code"
        >
          <span class="flag-icon">{{ c.flag }}</span>
          <span class="country-name">{{ c.name || c.code }}</span>
          <span class="country-cnt">{{ c.count }}</span>
        </button>
      </div>

      <!-- Toolbar Action & Settings Bar -->
      <div class="toolbar-bottom-row">
        <!-- Left: Probe Settings & Toggles -->
        <div class="probe-controls-group">
          <label class="toggle-checkbox" title="探测时一并执行小样本带宽测速 (约消耗 5MB 流量)">
            <input type="checkbox" v-model="includeSpeedtest" />
            <span>🚀 包含测速</span>
          </label>
          <label class="toggle-checkbox" title="探测时一并检测 ChatGPT、Gemini、YouTube、Netflix 等解锁状态">
            <input type="checkbox" v-model="includeMediaCheck" />
            <span>🎬 包含流媒体/AI</span>
          </label>
          <div class="speed-threshold-input" title="仅显示测速大于等于此阈值的节点">
            <span>门槛:</span>
            <input
              type="number"
              v-model.number="minSpeedThreshold"
              min="0"
              step="5"
              placeholder="0"
              class="mini-num-input"
            />
            <span>Mbps</span>
          </div>
        </div>

        <!-- Right: Actions & View Switcher -->
        <div class="view-and-actions">
          <!-- Selection helper -->
          <div class="selection-actions" v-if="selectedNodeNames.size > 0">
            <span class="selected-count">已选 <strong>{{ selectedNodeNames.size }}</strong> 项</span>
            <button class="primary small" @click="probeSelectedNodes" :disabled="probing">
              ⚡ 质检选中
            </button>
            <button class="small" @click="selectedNodeNames.clear()">取消选择</button>
          </div>
          <div class="selection-actions" v-else>
            <button
              class="small"
              @click="probeUntestedOrFailed"
              :disabled="probing || !untestedOrFailedCount"
              title="仅对未测试或之前失败的节点重新探测"
            >
              🎯 仅测未测/失败 ({{ untestedOrFailedCount }})
            </button>
            <button class="small danger-text" @click="clearProbeData" :disabled="clearing">
              🗑️ 清除质检数据
            </button>
          </div>

          <!-- View Mode Toggle -->
          <div class="view-toggle-seg">
            <button
              class="view-toggle-btn"
              :class="{ active: viewMode === 'grid' }"
              @click="viewMode = 'grid'"
              title="卡片网格模式"
            >
              🎴 卡片
            </button>
            <button
              class="view-toggle-btn"
              :class="{ active: viewMode === 'table' }"
              @click="viewMode = 'table'"
              title="高密度表格模式"
            >
              📋 表格
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Pager Toolbar -->
    <div class="pager-card">
      <div class="pager-left">
        <label class="select-all-label">
          <input
            type="checkbox"
            :checked="isAllCurrentPageSelected"
            @change="toggleSelectCurrentPage"
          />
          <span>当页全选 ({{ pagedRows.length }})</span>
        </label>
        <span class="muted count-indicator">共 {{ filtered.length }} / {{ rows.length }} 个节点</span>
      </div>

      <div class="pager-right">
        <button :disabled="page <= 1" @click="page--">上一页</button>
        <span class="muted pager-text">第 {{ normalizedPage }} / {{ totalPages }} 页</span>
        <button :disabled="page >= totalPages" @click="page++">下一页</button>
        <span class="muted">每页</span>
        <select v-model.number="pageSize" class="page-size-select">
          <option :value="24">24</option>
          <option :value="48">48</option>
          <option :value="96">96</option>
          <option :value="200">200</option>
        </select>
      </div>
    </div>

    <!-- Empty State -->
    <div v-if="loading && !rows.length" class="empty-state-card">
      <div class="spinner-inline large"></div>
      <p>正在拉取节点与探测数据…</p>
    </div>
    <div v-else-if="!filtered.length" class="empty-state-card">
      <p class="empty-emoji">🔍</p>
      <p>没有找到符合当前筛选条件的节点</p>
      <button class="primary small" @click="resetFilters">清空所有筛选条件</button>
    </div>

    <!-- Main Content: Card Grid View -->
    <div v-else-if="viewMode === 'grid'" class="node-cards-grid">
      <div
        v-for="item in pagedRows"
        :key="item.name"
        class="modern-node-card"
        :class="{
          selected: selectedNodeNames.has(item.name),
          is_ok: getProbe(item.name)?.status === 'ok',
          is_fail: getProbe(item.name)?.status === 'fail',
          is_timeout: getProbe(item.name)?.status === 'timeout',
        }"
      >
        <!-- Card Header -->
        <div class="card-head">
          <div class="card-head-left">
            <label class="item-checkbox-wrap" @click.stop>
              <input
                type="checkbox"
                :checked="selectedNodeNames.has(item.name)"
                @change="toggleSelectNode(item.name)"
              />
            </label>
            <span class="node-flag-icon">{{ getNodeFlag(item.name) }}</span>
            <span class="node-name-text" :title="item.name">{{ item.name }}</span>
          </div>
          <div class="card-head-right">
            <span class="proto-badge" :class="`proto-${(item.type || '').toLowerCase()}`">{{ item.type || 'RAW' }}</span>
          </div>
        </div>

        <!-- Meta info row -->
        <div class="card-meta-line">
          <span v-if="item.subscription_name" class="sub-tag" :title="`所属订阅: ${item.subscription_name}`">
            📁 {{ item.subscription_name }}
          </span>
          <span class="server-endpoint mono" :title="`${item.server}:${item.port}`">
            {{ item.server }}:{{ item.port }}
          </span>
        </div>

        <!-- Probe Status Metrics Banner -->
        <div class="probe-metrics-strip">
          <!-- Latency Badge -->
          <div
            v-if="getProbe(item.name)?.status === 'ok'"
            class="metric-pill pill-latency"
            :class="getLatencyClass(getProbe(item.name)?.latency_ms)"
            :title="`握手往返延迟: ${getProbe(item.name)?.latency_ms}ms`"
          >
            ⚡ {{ getProbe(item.name)?.latency_ms }}ms
          </div>
          <div
            v-else-if="getProbe(item.name)?.status === 'fail'"
            class="metric-pill pill-fail"
            :title="getProbe(item.name)?.error || '探测握手失败'"
          >
            🔴 失败
          </div>
          <div
            v-else-if="getProbe(item.name)?.status === 'timeout'"
            class="metric-pill pill-timeout"
            title="握手超时"
          >
            ⏱️ 超时
          </div>
          <div v-else class="metric-pill pill-untested" title="该节点尚未执行探测">
            ⚪ 未测
          </div>

          <!-- Speed Badge -->
          <div
            v-if="getProbe(item.name)?.speed_mbps"
            class="metric-pill pill-speed"
            :title="`测速带宽: ${getProbe(item.name)?.speed_mbps} Mbps`"
          >
            🚀 {{ getProbe(item.name)?.speed_mbps }}M
          </div>

          <!-- Outbound IP / Country -->
          <div
            v-if="getProbe(item.name)?.country || getProbe(item.name)?.ip"
            class="metric-pill pill-geo"
            :title="`真实出口 IP: ${getProbe(item.name)?.ip || '-'} | ASN: ${getProbe(item.name)?.asn || '-'} (${getProbe(item.name)?.organization || '-'})`"
          >
            🌍 {{ getProbe(item.name)?.country }} {{ getProbe(item.name)?.ip }}
          </div>
        </div>

        <!-- Media & AI Unlock Matrix on Card -->
        <div class="card-unlock-row" v-if="getProbe(item.name)?.media">
          <span
            v-for="p in mediaPlatformList"
            :key="p.key"
            class="media-chip-mini"
            :class="getMediaStatusClass(getProbe(item.name)?.media?.[p.key])"
            :title="`${p.name}: ${getMediaStatusLabel(getProbe(item.name)?.media?.[p.key])}`"
          >
            {{ p.short }}
          </span>
        </div>

        <!-- Dialer Chain Status -->
        <div class="card-chain-status" v-if="item.dialer_proxy">
          <span class="chain-indicator">
            🔗 dialer → <strong class="mono">{{ item.dialer_proxy }}</strong>
            <span class="chain-source-tag">({{ sourceLabel(item.chain_source) }})</span>
          </span>
        </div>

        <!-- Card Footer Actions -->
        <div class="card-actions">
          <button class="btn-card-action" @click="openDetailModal(item)" title="查看节点全部详细参数与真实诊断">
            🔍 详情
          </button>
          <button
            class="btn-card-action"
            @click="probeSingle(item)"
            :disabled="probingNodeKey === item.name"
            title="重新单独探测此节点"
          >
            {{ probingNodeKey === item.name ? '探测中…' : '⚡ 测此节点' }}
          </button>
          <button class="btn-card-action" @click="openChain(item)" title="为此节点绑定跳板前置代理">
            🔗 设跳板
          </button>
          <button
            v-if="item.dialer_proxy && item.chain_source === 'node'"
            class="btn-card-action danger-text"
            @click="clearNodeChain(item)"
            title="清除节点级跳板绑定"
          >
            清链
          </button>
        </div>
      </div>
    </div>

    <!-- Main Content: Table View -->
    <div v-else class="table-card node-table-card">
      <table class="rules-table node-table">
        <thead>
          <tr>
            <th class="col-check" style="width: 40px">
              <input
                type="checkbox"
                :checked="isAllCurrentPageSelected"
                @change="toggleSelectCurrentPage"
              />
            </th>
            <th class="col-name">节点名称</th>
            <th class="col-proto">协议</th>
            <th class="col-latency">延迟 / 状态</th>
            <th class="col-speed">测速</th>
            <th class="col-geo">出口地区 / IP</th>
            <th class="col-media">流媒体与 AI 解锁</th>
            <th class="col-sub">所属订阅</th>
            <th class="col-chain">跳板链路</th>
            <th class="col-actions">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in pagedRows"
            :key="item.name"
            :class="{
              selected: selectedNodeNames.has(item.name),
            }"
          >
            <!-- Checkbox -->
            <td class="col-check">
              <input
                type="checkbox"
                :checked="selectedNodeNames.has(item.name)"
                @change="toggleSelectNode(item.name)"
              />
            </td>

            <!-- Node Name -->
            <td class="col-name" :title="item.name">
              <div class="table-node-name-cell">
                <span class="node-flag-icon">{{ getNodeFlag(item.name) }}</span>
                <strong class="node-title-main">{{ item.name }}</strong>
              </div>
              <div class="table-node-subtext mono">{{ item.server }}:{{ item.port }}</div>
            </td>

            <!-- Protocol -->
            <td class="col-proto">
              <span class="proto-badge" :class="`proto-${(item.type || '').toLowerCase()}`">
                {{ item.type }}
              </span>
            </td>

            <!-- Latency / Status -->
            <td class="col-latency">
              <span
                v-if="getProbe(item.name)?.status === 'ok'"
                class="probe-pill probe-ok"
                :class="getLatencyClass(getProbe(item.name)?.latency_ms)"
                :title="`延迟: ${getProbe(item.name)?.latency_ms}ms`"
              >
                ⚡ {{ getProbe(item.name)?.latency_ms }}ms
              </span>
              <span
                v-else-if="getProbe(item.name)?.status === 'fail'"
                class="probe-pill probe-fail"
                :title="getProbe(item.name)?.error || '探测失败'"
              >
                🔴 失败
              </span>
              <span
                v-else-if="getProbe(item.name)?.status === 'timeout'"
                class="probe-pill probe-timeout"
                title="握手超时"
              >
                ⏱️ 超时
              </span>
              <span v-else class="muted" style="font-size: 11px">未测试</span>
            </td>

            <!-- Speed -->
            <td class="col-speed">
              <span
                v-if="getProbe(item.name)?.speed_mbps"
                class="probe-pill probe-speed"
                :title="`测速: ${getProbe(item.name)?.speed_mbps} Mbps`"
              >
                🚀 {{ getProbe(item.name)?.speed_mbps }}M
              </span>
              <span v-else class="muted">-</span>
            </td>

            <!-- Outbound IP / Geo -->
            <td class="col-geo">
              <template v-if="getProbe(item.name)?.country || getProbe(item.name)?.ip">
                <span class="geo-badge">
                  🌍 {{ getProbe(item.name)?.country }}
                </span>
                <span class="mono geo-ip-text" :title="getProbe(item.name)?.ip">
                  {{ getProbe(item.name)?.ip }}
                </span>
              </template>
              <span v-else class="muted">-</span>
            </td>

            <!-- Streaming & AI Unlock -->
            <td class="col-media">
              <div class="table-media-wrap" v-if="getProbe(item.name)?.media">
                <span
                  v-for="p in mediaPlatformList"
                  :key="p.key"
                  class="media-chip-mini"
                  :class="getMediaStatusClass(getProbe(item.name)?.media?.[p.key])"
                  :title="`${p.name}: ${getMediaStatusLabel(getProbe(item.name)?.media?.[p.key])}`"
                >
                  {{ p.short }}
                </span>
              </div>
              <span v-else class="muted">-</span>
            </td>

            <!-- Subscription -->
            <td class="col-sub muted" :title="item.subscription_name">
              {{ item.subscription_name || '-' }}
            </td>

            <!-- Chain -->
            <td class="col-chain">
              <template v-if="item.dialer_proxy">
                <span class="pill ok">via {{ item.dialer_proxy }}</span>
                <span v-if="item.chain_source" class="chain-src">{{ sourceLabel(item.chain_source) }}</span>
              </template>
              <span v-else class="muted">直连</span>
            </td>

            <!-- Actions -->
            <td class="col-actions">
              <div class="action-row compact-actions no-wrap">
                <button class="small" @click="openDetailModal(item)">详情</button>
                <button
                  class="small"
                  @click="probeSingle(item)"
                  :disabled="probingNodeKey === item.name"
                >
                  {{ probingNodeKey === item.name ? '…' : '⚡' }}
                </button>
                <button class="small" @click="openChain(item)">设跳板</button>
                <button
                  v-if="item.dialer_proxy && item.chain_source === 'node'"
                  class="small danger"
                  @click="clearNodeChain(item)"
                >
                  清链
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Node Detail & Diagnostics Modal / Drawer -->
    <div v-if="detailNode" class="modal-mask" @click.self="detailNode = null">
      <div class="modal-card detail-modal-card">
        <div class="modal-header-row">
          <div class="detail-head-left">
            <span class="node-flag-icon large">{{ getNodeFlag(detailNode.name) }}</span>
            <div>
              <h3>{{ detailNode.name }}</h3>
              <p class="section-hint mono">{{ detailNode.server }}:{{ detailNode.port }} · {{ detailNode.type?.toUpperCase() }}</p>
            </div>
          </div>
          <button class="modal-close-btn" @click="detailNode = null">✕</button>
        </div>

        <!-- Detail Tabs -->
        <div class="detail-tabs">
          <button
            class="detail-tab-btn"
            :class="{ active: detailTab === 'diagnostics' }"
            @click="detailTab = 'diagnostics'"
          >
            ⚡ 质检与诊断报告
          </button>
          <button
            class="detail-tab-btn"
            :class="{ active: detailTab === 'params' }"
            @click="detailTab = 'params'"
          >
            📋 网络与节点参数
          </button>
          <button
            class="detail-tab-btn"
            :class="{ active: detailTab === 'singbox' }"
            @click="detailTab = 'singbox'"
          >
            📦 Sing-box 配置预览
          </button>
          <button
            class="detail-tab-btn"
            :class="{ active: detailTab === 'clash' }"
            @click="detailTab = 'clash'"
          >
            ⚙️ Clash Proxy 格式
          </button>
        </div>

        <!-- Tab 1: Diagnostics -->
        <div v-if="detailTab === 'diagnostics'" class="detail-tab-content">
          <!-- Summary Cards -->
          <div class="diag-summary-grid">
            <div class="diag-card">
              <span class="diag-label">连通握手状态</span>
              <strong
                class="diag-val"
                :class="{
                  'text-ok': getProbe(detailNode.name)?.status === 'ok',
                  'text-danger': getProbe(detailNode.name)?.status === 'fail',
                }"
              >
                {{ getProbe(detailNode.name)?.status?.toUpperCase() || 'UNTESTED' }}
              </strong>
              <span class="diag-hint" v-if="getProbe(detailNode.name)?.latency_ms">
                往返延迟: {{ getProbe(detailNode.name)?.latency_ms }} ms
              </span>
            </div>

            <div class="diag-card">
              <span class="diag-label">真实出口 IP</span>
              <strong class="diag-val mono">{{ getProbe(detailNode.name)?.ip || '-' }}</strong>
              <span class="diag-hint">
                国家: {{ getProbe(detailNode.name)?.country || '-' }}
              </span>
            </div>

            <div class="diag-card">
              <span class="diag-label">实测带宽</span>
              <strong class="diag-val text-brand">{{ getProbe(detailNode.name)?.speed_mbps ? `${getProbe(detailNode.name)?.speed_mbps} Mbps` : '-' }}</strong>
              <span class="diag-hint">5MB 样本下行带宽</span>
            </div>

            <div class="diag-card">
              <span class="diag-label">ASN / 运营商</span>
              <strong class="diag-val">{{ getProbe(detailNode.name)?.asn ? `AS${getProbe(detailNode.name)?.asn}` : '-' }}</strong>
              <span class="diag-hint">{{ getProbe(detailNode.name)?.organization || '出口组织' }}</span>
            </div>
          </div>

          <!-- Streaming Unlock Full Table -->
          <h4 class="sub-section-title">🎬 流媒体与 AI 解锁平台详情</h4>
          <div class="media-detail-table-wrap">
            <table class="media-detail-table">
              <thead>
                <tr>
                  <th>平台</th>
                  <th>检测状态</th>
                  <th>等级 / 区域</th>
                  <th>详细说明</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="p in mediaPlatformList" :key="p.key">
                  <td>
                    <span class="platform-name-cell">
                      <span>{{ p.icon }}</span>
                      <strong>{{ p.name }}</strong>
                    </span>
                  </td>
                  <td>
                    <span class="probe-pill" :class="getMediaStatusClass(getProbe(detailNode.name)?.media?.[p.key])">
                      {{ getMediaStatusLabel(getProbe(detailNode.name)?.media?.[p.key]) }}
                    </span>
                  </td>
                  <td class="mono">
                    {{ getProbe(detailNode.name)?.media?.[p.key]?.region || getProbe(detailNode.name)?.media?.[p.key]?.label || '-' }}
                  </td>
                  <td class="muted text-small">
                    {{ getMediaDetailDesc(p.key, getProbe(detailNode.name)?.media?.[p.key]) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="diag-actions-bar">
            <button class="primary" @click="probeSingle(detailNode)" :disabled="probingNodeKey === detailNode.name">
              {{ probingNodeKey === detailNode.name ? '正在深度探测…' : '⚡ 重新深度探测此节点' }}
            </button>
            <span class="muted text-small" v-if="getProbe(detailNode.name)?.checked_at">
              最后质检时间: {{ formatTimestamp(getProbe(detailNode.name)?.checked_at) }}
            </span>
          </div>
        </div>

        <!-- Tab 2: Raw Parameters -->
        <div v-else-if="detailTab === 'params'" class="detail-tab-content">
          <div class="params-grid">
            <div class="param-row" v-for="(val, key) in detailNodeParams" :key="key">
              <span class="param-key mono">{{ key }}</span>
              <span class="param-val mono">{{ typeof val === 'object' ? JSON.stringify(val) : val }}</span>
            </div>
          </div>
        </div>

        <!-- Tab 3: Sing-box Config -->
        <div v-else-if="detailTab === 'singbox'" class="detail-tab-content">
          <div class="code-preview-wrap">
            <div class="code-header">
              <span>Sing-box Outbound / Endpoint JSON</span>
              <button class="small" @click="copyText(singboxJsonPreview)">📋 复制 JSON</button>
            </div>
            <pre class="code-block mono">{{ singboxJsonPreview }}</pre>
          </div>
        </div>

        <!-- Tab 4: Clash Config -->
        <div v-else-if="detailTab === 'clash'" class="detail-tab-content">
          <div class="code-preview-wrap">
            <div class="code-header">
              <span>Clash / Mihomo Proxy Dict (JSON)</span>
              <button class="small" @click="copyText(clashJsonPreview)">📋 复制配置</button>
            </div>
            <pre class="code-block mono">{{ clashJsonPreview }}</pre>
          </div>
        </div>
      </div>
    </div>

    <!-- Set Dialer Chain Modal -->
    <div v-if="chainTarget" class="modal-mask" @click.self="closeChain">
      <div class="modal-card">
        <h3>给节点设跳板前置代理</h3>
        <p class="section-hint mono target-name">{{ chainTarget.name }}</p>

        <div class="seg" style="margin: 14px 0">
          <button
            type="button"
            :class="{ active: chainForm.dialer_type === 'node' }"
            @click="chainForm.dialer_type = 'node'; chainForm.dialer_ref = ''"
          >
            节点
          </button>
          <button
            type="button"
            :class="{ active: chainForm.dialer_type === 'node_group' }"
            @click="chainForm.dialer_type = 'node_group'; chainForm.dialer_ref = ''"
          >
            策略组
          </button>
        </div>

        <label v-if="chainForm.dialer_type === 'node'" class="field">
          <span>跳板节点</span>
          <input v-model="dialerSearch" placeholder="搜索节点名称…" />
          <select v-model="chainForm.dialer_ref">
            <option value="">选择跳板节点</option>
            <option v-for="n in dialerNodeOptions" :key="n.name" :value="n.name">{{ n.name }}</option>
          </select>
        </label>
        <label v-else class="field">
          <span>跳板策略组</span>
          <input v-model="dialerSearch" placeholder="搜索策略组…" />
          <select v-model="chainForm.dialer_ref">
            <option value="">选择策略组</option>
            <option v-for="g in dialerGroupOptions" :key="g.id" :value="g.name">{{ g.name }}</option>
          </select>
        </label>

        <div class="template-actions sheet-actions" style="margin-top: 18px">
          <button class="primary" @click="saveChain" :disabled="saving || !chainForm.dialer_ref">
            {{ saving ? '保存中…' : '保存跳板绑定' }}
          </button>
          <button @click="closeChain">取消</button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { getNodeFlag } from '../utils/format'
import {
  clearProbeResults,
  createProxyChain,
  deleteProxyChain,
  getApiErrorMessage,
  getNodeGroups,
  getNodeLedger,
  getProbeResults,
  getProxyChains,
  probeNode,
  probeNodesFull,
} from '../api'
import { useAppStore } from '../stores/app'
import { useUrlState } from '../utils/urlState'

const store = useAppStore()

// State
const loading = ref(false)
const saving = ref(false)
const clearing = ref(false)
const error = ref('')
const rows = ref([])
const bindings = ref([])
const nodeGroups = ref([])

// Filters
const search = useUrlState('q', '')
const subFilter = useUrlState('sub', '')
const typeFilter = useUrlState('type', '')
const statusFilter = ref('all')
const countryFilter = ref('')
const chainFilter = ref('all')
const sortBy = ref('default')
const selectedMediaFilters = ref([])
const minSpeedThreshold = ref(0)
const speedFilter = ref(0)

// View & Pagination
const viewMode = ref('grid') // 'grid' | 'table'
const page = ref(1)
const pageSize = ref(48)
const selectedNodeNames = reactive(new Set())

// Probe Options & Execution State
const includeSpeedtest = ref(false)
const includeMediaCheck = ref(true)
const probing = ref(false)
const probingNodeKey = ref(null)
const probeResults = ref({})
const probeProgress = reactive({
  done: 0,
  total: 0,
  ok: 0,
  fail: 0,
})
let probeAbortController = null

// Modals
const chainTarget = ref(null)
const dialerSearch = ref('')
const chainForm = reactive({
  dialer_type: 'node',
  dialer_ref: '',
})

// Detail Modal
const detailNode = ref(null)
const detailTab = ref('diagnostics')

// Media Platforms Definition
const mediaPlatformList = [
  { key: 'chatgpt', name: 'ChatGPT', short: 'GPT', icon: '🤖' },
  { key: 'gemini', name: 'Gemini', short: 'Gemini', icon: '🧠' },
  { key: 'youtube', name: 'YouTube', short: 'YT', icon: '📺' },
  { key: 'netflix', name: 'Netflix', short: 'NF', icon: '🍿' },
  { key: 'disney', name: 'Disney+', short: 'Disney', icon: '🏰' },
  { key: 'meta_ai', name: 'Meta AI', short: 'Meta', icon: '🌐' },
  { key: 'bilibili', name: 'Bilibili', short: 'Bili', icon: '⚡' },
]

// Country name map
const countryNameMap = {
  HK: '香港',
  TW: '台湾',
  JP: '日本',
  SG: '新加坡',
  US: '美国',
  KR: '韩国',
  DE: '德国',
  GB: '英国',
  FR: '法国',
  CA: '加拿大',
  AU: '澳大利亚',
  CN: '中国大陆',
}

const countryFlagMap = {
  HK: '🇭🇰',
  TW: '🇹🇼',
  JP: '🇯🇵',
  SG: '🇸🇬',
  US: '🇺🇸',
  KR: '🇰🇷',
  DE: '🇩🇪',
  GB: '🇬🇧',
  FR: '🇫🇷',
  CA: '🇨🇦',
  AU: '🇦🇺',
  CN: '🇨🇳',
}

// Computed Options
const subOptions = computed(() => {
  const set = new Set()
  for (const row of rows.value) {
    if (row.subscription_name) set.add(row.subscription_name)
  }
  return [...set].sort((a, b) => a.localeCompare(b, 'zh'))
})

const typeOptions = computed(() => {
  const set = new Set()
  for (const row of rows.value) {
    if (row.type) set.add(row.type)
  }
  return [...set].sort()
})

const chainedCount = computed(() => rows.value.filter((r) => r.dialer_proxy).length)

// Media stats
const mediaStats = computed(() => {
  const stats = {}
  for (const p of mediaPlatformList) {
    stats[p.key] = 0
  }
  for (const row of rows.value) {
    const probe = probeResults.value[row.name] || probeResults.value[row.node_key]
    if (probe?.media) {
      for (const p of mediaPlatformList) {
        const item = probe.media[p.key]
        if (item?.status === 'ok' || item?.status === 'full' || item?.status === 'originals' || item?.unlocked) {
          stats[p.key]++
        }
      }
    }
  }
  return stats
})

// Country breakdown
const countryOptions = computed(() => {
  const counts = {}
  for (const r of rows.value) {
    const probe = probeResults.value[r.name] || probeResults.value[r.node_key]
    let code = (probe?.country || '').toUpperCase()
    if (!code) {
      // Fallback: match from node name
      const n = (r.name || '').toUpperCase()
      if (n.includes('香港') || n.includes('HK')) code = 'HK'
      else if (n.includes('日本') || n.includes('JP')) code = 'JP'
      else if (n.includes('美国') || n.includes('US')) code = 'US'
      else if (n.includes('新加坡') || n.includes('SG')) code = 'SG'
      else if (n.includes('台湾') || n.includes('TW')) code = 'TW'
      else if (n.includes('韩国') || n.includes('KR')) code = 'KR'
      else if (n.includes('德国') || n.includes('DE')) code = 'DE'
      else if (n.includes('英国') || n.includes('GB') || n.includes('UK')) code = 'GB'
    }
    if (code) {
      counts[code] = (counts[code] || 0) + 1
    }
  }
  return Object.entries(counts)
    .map(([code, count]) => ({
      code,
      name: countryNameMap[code] || code,
      flag: countryFlagMap[code] || '🌐',
      count,
    }))
    .sort((a, b) => b.count - a.count)
})

// Health metrics
const testedCount = computed(() => {
  return rows.value.filter((r) => {
    const p = probeResults.value[r.name] || probeResults.value[r.node_key]
    return p && p.status && p.status !== 'untested'
  }).length
})

const healthyCount = computed(() => {
  return rows.value.filter((r) => {
    const p = probeResults.value[r.name] || probeResults.value[r.node_key]
    return p?.status === 'ok'
  }).length
})

const healthPercent = computed(() => {
  if (!testedCount.value) return 0
  return Math.round((healthyCount.value / testedCount.value) * 100)
})

const healthRateClass = computed(() => {
  if (!testedCount.value) return 'text-muted'
  if (healthPercent.value >= 80) return 'text-ok'
  if (healthPercent.value >= 50) return 'text-warning'
  return 'text-danger'
})

const avgLatency = computed(() => {
  const okList = rows.value
    .map((r) => (probeResults.value[r.name] || probeResults.value[r.node_key])?.latency_ms)
    .filter((ms) => typeof ms === 'number' && ms > 0)
  if (!okList.length) return null
  return Math.round(okList.reduce((a, b) => a + b, 0) / okList.length)
})

const testedSpeedCount = computed(() => {
  return rows.value.filter((r) => {
    const p = probeResults.value[r.name] || probeResults.value[r.node_key]
    return typeof p?.speed_mbps === 'number' && p.speed_mbps > 0
  }).length
})

const highSpeedCount = computed(() => {
  return rows.value.filter((r) => {
    const p = probeResults.value[r.name] || probeResults.value[r.node_key]
    return typeof p?.speed_mbps === 'number' && p.speed_mbps >= 10.0
  }).length
})

const maxSpeed = computed(() => {
  const speeds = rows.value
    .map((r) => (probeResults.value[r.name] || probeResults.value[r.node_key])?.speed_mbps)
    .filter((s) => typeof s === 'number' && s > 0)
  if (!speeds.length) return null
  return Math.max(...speeds)
})

const untestedOrFailedCount = computed(() => {
  return rows.value.filter((r) => {
    const p = probeResults.value[r.name] || probeResults.value[r.node_key]
    return !p || p.status !== 'ok'
  }).length
})

const probeProgressPercent = computed(() => {
  if (!probeProgress.total) return 0
  return Math.min(100, Math.round((probeProgress.done / probeProgress.total) * 100))
})

// Filter Pipeline
const filtered = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()

  return (rows.value || []).filter((item) => {
    const probe = probeResults.value[item.name] || probeResults.value[item.node_key]

    // Subscription
    if (subFilter.value && item.subscription_name !== subFilter.value) return false

    // Protocol
    if (typeFilter.value && item.type !== typeFilter.value) return false

    // Status Filter
    if (statusFilter.value === 'ok' && probe?.status !== 'ok') return false
    if (statusFilter.value === 'fast' && (probe?.status !== 'ok' || !probe?.latency_ms || probe.latency_ms > 300)) return false
    if (statusFilter.value === 'medium' && (probe?.status !== 'ok' || !probe?.latency_ms || probe.latency_ms <= 300 || probe.latency_ms > 800)) return false
    if (statusFilter.value === 'fail' && probe?.status !== 'fail' && probe?.status !== 'timeout') return false
    if (statusFilter.value === 'untested' && probe?.status && probe.status !== 'untested') return false

    // Country Filter
    if (countryFilter.value) {
      const c = (probe?.country || '').toUpperCase()
      const n = (item.name || '').toUpperCase()
      const matchesName =
        (countryFilter.value === 'HK' && (n.includes('香港') || n.includes('HK'))) ||
        (countryFilter.value === 'JP' && (n.includes('日本') || n.includes('JP'))) ||
        (countryFilter.value === 'US' && (n.includes('美国') || n.includes('US'))) ||
        (countryFilter.value === 'SG' && (n.includes('新加坡') || n.includes('SG'))) ||
        (countryFilter.value === 'TW' && (n.includes('台湾') || n.includes('TW'))) ||
        (countryFilter.value === 'KR' && (n.includes('韩国') || n.includes('KR'))) ||
        (countryFilter.value === 'DE' && (n.includes('德国') || n.includes('DE'))) ||
        (countryFilter.value === 'GB' && (n.includes('英国') || n.includes('GB') || n.includes('UK')))
      if (c !== countryFilter.value && !matchesName) return false
    }

    // Chain filter
    if (chainFilter.value === 'chained' && !item.dialer_proxy) return false
    if (chainFilter.value === 'plain' && item.dialer_proxy) return false

    // Speed Threshold
    const minSpeed = Math.max(minSpeedThreshold.value || 0, speedFilter.value || 0)
    if (minSpeed > 0 && (!probe?.speed_mbps || probe.speed_mbps < minSpeed)) return false

    // Media unlock filters (AND logic)
    if (selectedMediaFilters.value.length) {
      for (const mediaKey of selectedMediaFilters.value) {
        const m = probe?.media?.[mediaKey]
        if (!m || !(m.status === 'ok' || m.status === 'full' || m.status === 'originals' || m.unlocked)) {
          return false
        }
      }
    }

    // Search query
    if (!q) return true
    const hay = [
      item.name,
      item.subscription_name,
      item.dialer_proxy,
      item.type,
      item.server,
      item.port,
      item.cipher,
      item.network,
      item.sni,
      probe?.ip,
      probe?.country,
      probe?.asn ? `AS${probe.asn}` : '',
      probe?.organization,
      ...(item.group_names || []),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()

    return hay.includes(q)
  }).sort((a, b) => {
    const pA = probeResults.value[a.name] || probeResults.value[a.node_key]
    const pB = probeResults.value[b.name] || probeResults.value[b.node_key]

    if (sortBy.value === 'latency_asc') {
      const latA = pA?.status === 'ok' ? pA.latency_ms || 99999 : 999999
      const latB = pB?.status === 'ok' ? pB.latency_ms || 99999 : 999999
      return latA - latB
    }
    if (sortBy.value === 'latency_desc') {
      const latA = pA?.status === 'ok' ? pA.latency_ms || 0 : -1
      const latB = pB?.status === 'ok' ? pB.latency_ms || 0 : -1
      return latB - latA
    }
    if (sortBy.value === 'speed_desc') {
      const spA = pA?.speed_mbps || 0
      const spB = pB?.speed_mbps || 0
      return spB - spA
    }
    if (sortBy.value === 'name_asc') {
      return (a.name || '').localeCompare(b.name || '', 'zh')
    }
    if (sortBy.value === 'country') {
      return (pA?.country || '').localeCompare(pB?.country || '')
    }
    if (sortBy.value === 'checked_desc') {
      return (pB?.checked_at || 0) - (pA?.checked_at || 0)
    }
    return 0
  })
})

// Pagination
const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize.value)))
const normalizedPage = computed(() => Math.min(page.value, totalPages.value))
const pagedRows = computed(() =>
  filtered.value.slice((normalizedPage.value - 1) * pageSize.value, normalizedPage.value * pageSize.value),
)

// Select all on page
const isAllCurrentPageSelected = computed(() => {
  if (!pagedRows.value.length) return false
  return pagedRows.value.every((r) => selectedNodeNames.has(r.name))
})

const filteredOrSelectedNodes = computed(() => {
  if (selectedNodeNames.size > 0) {
    return rows.value.filter((r) => selectedNodeNames.has(r.name))
  }
  return filtered.value
})

watch([search, subFilter, typeFilter, statusFilter, countryFilter, chainFilter, sortBy, pageSize, minSpeedThreshold, selectedMediaFilters], () => {
  page.value = 1
})

onMounted(reload)

// Methods
function getProbe(name) {
  return probeResults.value[name] || {}
}

function getLatencyClass(ms) {
  if (typeof ms !== 'number' || ms <= 0) return ''
  if (ms < 300) return 'lat-fast'
  if (ms < 800) return 'lat-medium'
  return 'lat-slow'
}

function getMediaStatusClass(m) {
  if (!m) return 'm-none'
  if (m.status === 'ok' || m.status === 'full' || m.unlocked) return 'm-ok'
  if (m.status === 'originals') return 'm-partial'
  if (m.status === 'blocked' || m.status === 'fail') return 'm-blocked'
  return 'm-none'
}

function getMediaStatusLabel(m) {
  if (!m) return '未测试'
  if (m.status === 'ok' || m.unlocked) return '解锁正常'
  if (m.status === 'full') return '原生全解锁'
  if (m.status === 'originals') return '仅自制剧'
  if (m.status === 'blocked') return '地区受限/阻断'
  if (m.status === 'fail') return '测试失败'
  return '未知'
}

function getMediaDetailDesc(platformKey, m) {
  if (!m) return '尚未执行出站探测'
  if (m.status === 'ok') return `通过代理成功建立 TLS 访问，未遭遇区域风控。`
  if (m.status === 'full') return `已成功访问非自制版权片库（Breaking Bad），享有完整原生体验。`
  if (m.status === 'originals') return `仅可观看自制剧（Stranger Things），非自制片库受限。`
  if (m.status === 'blocked') return `遭遇平台地区阻断或 IP 风控限制。`
  if (m.error) return `请求异常: ${m.error}`
  return '-'
}

function sourceLabel(src) {
  if (src === 'node') return '节点绑定'
  if (src === 'node_group') return '策略组'
  if (src === 'subscription') return '订阅'
  return src || ''
}

function formatTimestamp(ts) {
  if (!ts) return '-'
  const d = new Date(ts * 1000)
  return d.toLocaleString('zh-CN', { hour12: false })
}

function toggleSelectNode(name) {
  if (selectedNodeNames.has(name)) {
    selectedNodeNames.delete(name)
  } else {
    selectedNodeNames.add(name)
  }
}

function toggleSelectCurrentPage() {
  if (isAllCurrentPageSelected.value) {
    for (const r of pagedRows.value) {
      selectedNodeNames.delete(r.name)
    }
  } else {
    for (const r of pagedRows.value) {
      selectedNodeNames.add(r.name)
    }
  }
}

function toggleMediaFilter(key) {
  const idx = selectedMediaFilters.value.indexOf(key)
  if (idx >= 0) {
    selectedMediaFilters.value.splice(idx, 1)
  } else {
    selectedMediaFilters.value.push(key)
  }
}

function toggleStatusFilter(val) {
  statusFilter.value = statusFilter.value === val ? 'all' : val
}

function toggleSpeedFilter(mbps) {
  speedFilter.value = speedFilter.value === mbps ? 0 : mbps
}

function toggleChainFilter() {
  chainFilter.value = chainFilter.value === 'chained' ? 'all' : 'chained'
}

function resetFilters() {
  search.value = ''
  subFilter.value = ''
  typeFilter.value = ''
  statusFilter.value = 'all'
  countryFilter.value = ''
  chainFilter.value = 'all'
  sortBy.value = 'default'
  selectedMediaFilters.value = []
  minSpeedThreshold.value = 0
  speedFilter.value = 0
  selectedNodeNames.clear()
}

// Data Load
async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [ledger, chains, groups, probeRes] = await Promise.all([
      getNodeLedger(),
      getProxyChains(),
      getNodeGroups(),
      getProbeResults().catch(() => ({ data: {} })),
    ])
    rows.value = ledger.data || []
    bindings.value = chains.data || []
    nodeGroups.value = groups.data || []

    const resData = probeRes?.data?.results || probeRes?.data
    if (resData && typeof resData === 'object') {
      probeResults.value = { ...resData }
    }
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载节点列表失败')
  } finally {
    loading.value = false
  }
}

// Single node probe
async function probeSingle(item) {
  if (!item || probingNodeKey.value) return
  probingNodeKey.value = item.name
  error.value = ''
  try {
    const { data } = await probeNode({
      node: item,
      include_speed: includeSpeedtest.value,
      include_media: includeMediaCheck.value,
      use_cache: false,
    })
    if (data) {
      probeResults.value = {
        ...probeResults.value,
        [item.name]: data,
        [data.node_key]: data,
      }
    }
  } catch (err) {
    store.toast(getApiErrorMessage(err, '节点探测失败'), 'error')
  } finally {
    probingNodeKey.value = null
  }
}

// Batch Probe
async function startProbeBatch(nodesToProbe) {
  if (!nodesToProbe || !nodesToProbe.length || probing.value) return
  probing.value = true
  error.value = ''
  probeProgress.done = 0
  probeProgress.total = nodesToProbe.length
  probeProgress.ok = 0
  probeProgress.fail = 0

  // Run in chunks for live UI feedback
  const CHUNK_SIZE = 10
  const chunks = []
  for (let i = 0; i < nodesToProbe.length; i += CHUNK_SIZE) {
    chunks.push(nodesToProbe.slice(i, i + CHUNK_SIZE))
  }

  try {
    for (const chunk of chunks) {
      if (!probing.value) break // User cancelled

      const res = await probeNodesFull({
        nodes: chunk,
        include_speed: includeSpeedtest.value,
        include_media: includeMediaCheck.value,
        concurrency: 5,
        use_cache: false,
      })

      const list = res?.data?.results || []
      for (const item of list) {
        if (item.status === 'ok') probeProgress.ok++
        else probeProgress.fail++
        probeProgress.done++

        if (item.name) {
          probeResults.value[item.name] = item
        }
        if (item.node_key) {
          probeResults.value[item.node_key] = item
        }
      }
      probeResults.value = { ...probeResults.value }
    }
    store.toast(`质检完成：${probeProgress.ok} 正常，${probeProgress.fail} 异常`, 'success')
  } catch (err) {
    error.value = getApiErrorMessage(err, '综合质检执行失败')
  } finally {
    probing.value = false
  }
}

function cancelProbeBatch() {
  probing.value = false
  store.toast('已停止后续探测任务', 'info')
}

function probeSelectedNodes() {
  const nodes = rows.value.filter((r) => selectedNodeNames.has(r.name))
  startProbeBatch(nodes)
}

function probeUntestedOrFailed() {
  const targets = rows.value.filter((r) => {
    const p = probeResults.value[r.name] || probeResults.value[r.node_key]
    return !p || p.status !== 'ok'
  })
  startProbeBatch(targets)
}

async function clearProbeData() {
  const ok = await store.confirm({
    title: '清空质检数据',
    message: '确定要清空所有已持久化的探测、测速与流媒体解锁记录吗？',
    confirmText: '清空',
    danger: true,
  })
  if (!ok) return

  clearing.value = true
  try {
    await clearProbeResults()
    probeResults.value = {}
    store.toast('已清空全部质检记录', 'success')
  } catch (err) {
    store.toast(getApiErrorMessage(err, '清空失败'), 'error')
  } finally {
    clearing.value = false
  }
}

// Modals: Detail
function openDetailModal(item) {
  detailNode.value = item
  detailTab.value = 'diagnostics'
}

const detailNodeParams = computed(() => {
  if (!detailNode.value) return {}
  const clean = { ...detailNode.value }
  delete clean.group_names
  delete clean.subscription_name
  return clean
})

const singboxJsonPreview = computed(() => {
  if (!detailNode.value) return ''
  const item = detailNode.value
  const proto = (item.type || '').toLowerCase()
  return JSON.stringify(
    {
      type: proto,
      tag: 'proxy-out',
      server: item.server,
      server_port: item.port,
      uuid: item.uuid,
      password: item.password ? '***' : undefined,
      tls: item.tls ? { enabled: true, server_name: item.sni || item.servername } : undefined,
    },
    null,
    2,
  )
})

const clashJsonPreview = computed(() => {
  if (!detailNode.value) return ''
  return JSON.stringify(detailNode.value, null, 2)
})

function copyText(txt) {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  store.toast('已复制到剪贴板', 'success')
}

// Modals: Chain
const dialerNodeOptions = computed(() => {
  const q = String(dialerSearch.value || '').trim().toLowerCase()
  const list = (rows.value || []).filter((n) => n.name !== chainTarget.value?.name)
  if (!q) return list
  return list.filter((n) => String(n.name || '').toLowerCase().includes(q))
})

const dialerGroupOptions = computed(() => {
  const q = String(dialerSearch.value || '').trim().toLowerCase()
  const list = nodeGroups.value || []
  if (!q) return list
  return list.filter((g) => String(g.name || '').toLowerCase().includes(q))
})

function openChain(item) {
  chainTarget.value = item
  chainForm.dialer_type = 'node'
  chainForm.dialer_ref = ''
  dialerSearch.value = ''
  error.value = ''
}

function closeChain() {
  chainTarget.value = null
  chainForm.dialer_ref = ''
}

async function saveChain() {
  if (!chainTarget.value || !chainForm.dialer_ref || saving.value) return
  saving.value = true
  error.value = ''
  try {
    const existing = (bindings.value || []).filter(
      (b) => b.target_type === 'node' && b.target_name === chainTarget.value.name,
    )
    await Promise.allSettled(existing.map((b) => deleteProxyChain(b.id)))
    await createProxyChain({
      target_type: 'node',
      target_name: chainTarget.value.name,
      dialer_type: chainForm.dialer_type,
      dialer_ref: chainForm.dialer_ref,
      enabled: true,
      note: 'from node management',
    })
    closeChain()
    await reload()
    store.toast('跳板绑定成功', 'success')
  } catch (err) {
    error.value = getApiErrorMessage(err, '设跳板失败')
  } finally {
    saving.value = false
  }
}

async function clearNodeChain(item) {
  const ok = await store.confirm({
    title: '清除节点跳板',
    message: `确定要清除节点「${item.name}」的跳板绑定吗？`,
    confirmText: '清除',
    danger: true,
  })
  if (!ok) return
  error.value = ''
  try {
    const existing = (bindings.value || []).filter(
      (b) => b.target_type === 'node' && b.target_name === item.name,
    )
    await Promise.allSettled(existing.map((b) => deleteProxyChain(b.id)))
    await reload()
    store.toast('已清除节点跳板', 'success')
  } catch (err) {
    error.value = getApiErrorMessage(err, '清链失败')
  }
}
</script>

<style scoped>
.nodes-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Header & Glow Button */
.btn-glow {
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.35);
}

/* Progress Banner */
.probe-progress-card {
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.12), rgba(20, 184, 166, 0.1));
  border: 1px solid var(--brand);
  border-radius: var(--radius-md);
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.progress-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}
.progress-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}
.progress-nums {
  color: var(--brand);
  font-weight: bold;
}
.progress-stats {
  display: flex;
  gap: 8px;
}
.stat-tag {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 6px;
  font-weight: 600;
}
.stat-ok {
  background: rgba(15, 138, 95, 0.15);
  color: var(--ok);
}
.stat-fail {
  background: rgba(220, 38, 38, 0.15);
  color: var(--danger);
}
.stat-speed {
  background: rgba(37, 99, 235, 0.15);
  color: var(--brand);
}
.progress-bar-bg {
  width: 100%;
  height: 6px;
  background: rgba(0, 0, 0, 0.08);
  border-radius: 99px;
  overflow: hidden;
}
.progress-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--brand), var(--brand-2));
  transition: width 0.3s ease;
}

/* Insights Header Grid */
.insights-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}
.metric-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}
.metric-card:hover {
  transform: translateY(-2px);
  border-color: var(--brand);
  box-shadow: var(--shadow-sm);
}
.metric-card.active {
  border-color: var(--brand);
  background: linear-gradient(180deg, var(--surface), rgba(37, 99, 235, 0.06));
}
.metric-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.metric-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ink-soft);
}
.metric-icon {
  font-size: 16px;
}
.metric-val {
  font-size: 22px;
  font-weight: 800;
  line-height: 1.2;
}
.metric-sub {
  font-size: 12px;
  color: var(--ink-soft);
}

/* Media Matrix Card */
.media-matrix-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.matrix-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.matrix-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--ink);
}
.matrix-hint {
  font-size: 12px;
  color: var(--ink-soft);
}
.matrix-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.matrix-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 99px;
  font-size: 12px;
  cursor: pointer;
  user-select: none;
  transition: all 0.15s ease;
}
.matrix-chip:hover {
  border-color: var(--brand);
  background: rgba(37, 99, 235, 0.05);
}
.matrix-chip.active {
  background: var(--brand);
  border-color: var(--brand);
  color: #fff;
}
.matrix-chip.active .chip-count {
  background: rgba(255, 255, 255, 0.25);
  color: #fff;
}
.matrix-chip.has_unlocked .chip-count {
  color: var(--ok);
  font-weight: 800;
}
.chip-count {
  font-size: 11px;
  padding: 1px 6px;
  background: rgba(0, 0, 0, 0.05);
  border-radius: 99px;
}

/* Control Panel & Toolbar */
.control-panel-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.filter-row.main-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}
.search-input-wrap {
  flex: 1 1 260px;
  position: relative;
  display: flex;
  align-items: center;
}
.search-icon {
  position: absolute;
  left: 10px;
  font-size: 14px;
  opacity: 0.6;
}
.ledger-search-input {
  width: 100%;
  padding-left: 32px;
  padding-right: 28px;
  height: 36px;
  border-radius: 8px;
}
.clear-btn {
  position: absolute;
  right: 6px;
  background: transparent;
  border: none;
  color: var(--ink-soft);
  cursor: pointer;
  padding: 4px;
}
.filter-select {
  height: 36px;
  border-radius: 8px;
  min-width: 130px;
  flex: 0 1 auto;
}
.sort-select {
  border-color: var(--border-strong);
}

/* Country Pills */
.country-pills-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  padding-top: 4px;
  border-top: 1px dashed var(--border);
}
.pills-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ink-soft);
  margin-right: 4px;
}
.country-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  font-size: 12px;
  border-radius: 99px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  cursor: pointer;
}
.country-pill:hover {
  border-color: var(--brand);
}
.country-pill.active {
  background: var(--brand);
  color: #fff;
  border-color: var(--brand);
}
.country-pill.active .country-cnt {
  color: #fff;
  opacity: 0.8;
}
.country-cnt {
  font-size: 11px;
  opacity: 0.7;
}

/* Toolbar Bottom Actions */
.toolbar-bottom-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  padding-top: 4px;
}
.probe-controls-group {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
}
.toggle-checkbox {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  cursor: pointer;
  user-select: none;
}
.speed-threshold-input {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--ink-soft);
}
.mini-num-input {
  width: 54px;
  height: 28px;
  padding: 2px 6px;
  font-size: 12px;
  border-radius: 6px;
}
.view-and-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.selection-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.selected-count {
  font-size: 12px;
}
.view-toggle-seg {
  display: inline-flex;
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
}
.view-toggle-btn {
  border: none;
  background: var(--surface-2);
  padding: 6px 12px;
  font-size: 12px;
  border-radius: 0;
  cursor: pointer;
}
.view-toggle-btn.active {
  background: var(--brand);
  color: #fff;
}

/* Pager Card */
.pager-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  padding: 10px 14px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}
.pager-left,
.pager-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
.select-all-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  cursor: pointer;
}
.count-indicator {
  font-size: 12px;
}
.pager-text {
  font-size: 13px;
}

/* Modern Card Grid */
.node-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}
.modern-node-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: all 0.2s ease;
  position: relative;
}
.modern-node-card:hover {
  border-color: var(--brand);
  box-shadow: var(--shadow-sm);
}
.modern-node-card.selected {
  border-color: var(--brand);
  background: linear-gradient(180deg, var(--surface), rgba(37, 99, 235, 0.04));
}
.modern-node-card.is_ok {
  border-left: 4px solid var(--ok);
}
.modern-node-card.is_fail {
  border-left: 4px solid var(--danger);
}
.modern-node-card.is_timeout {
  border-left: 4px solid var(--warning);
}

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 8px;
}
.card-head-left {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
}
.node-name-text {
  font-size: 14px;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.proto-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 6px;
  font-weight: 700;
  text-transform: uppercase;
  background: var(--surface-2);
  border: 1px solid var(--border);
}
.proto-vless {
  background: rgba(147, 51, 234, 0.12);
  color: #9333ea;
  border-color: rgba(147, 51, 234, 0.2);
}
.proto-hysteria2,
.proto-hy2 {
  background: rgba(37, 99, 235, 0.12);
  color: #2563eb;
  border-color: rgba(37, 99, 235, 0.2);
}
.proto-vmess {
  background: rgba(245, 158, 11, 0.12);
  color: #d97706;
  border-color: rgba(245, 158, 11, 0.2);
}
.proto-ss,
.proto-shadowsocks {
  background: rgba(16, 185, 129, 0.12);
  color: #059669;
  border-color: rgba(16, 185, 129, 0.2);
}
.proto-wireguard {
  background: rgba(236, 72, 153, 0.12);
  color: #db2777;
  border-color: rgba(236, 72, 153, 0.2);
}
.proto-trojan {
  background: rgba(6, 182, 212, 0.12);
  color: #0891b2;
  border-color: rgba(6, 182, 212, 0.2);
}

.card-meta-line {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  color: var(--ink-soft);
}
.sub-tag {
  background: var(--surface-2);
  padding: 2px 6px;
  border-radius: 4px;
}
.server-endpoint {
  font-size: 11px;
}

/* Probe Metrics Strip */
.probe-metrics-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.metric-pill {
  font-size: 12px;
  padding: 3px 8px;
  border-radius: 6px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.pill-latency {
  background: rgba(15, 138, 95, 0.1);
  color: var(--ok);
}
.pill-latency.lat-fast {
  background: rgba(15, 138, 95, 0.15);
  color: var(--ok);
}
.pill-latency.lat-medium {
  background: rgba(245, 158, 11, 0.15);
  color: #d97706;
}
.pill-latency.lat-slow {
  background: rgba(220, 38, 38, 0.15);
  color: var(--danger);
}
.pill-fail {
  background: rgba(220, 38, 38, 0.15);
  color: var(--danger);
}
.pill-timeout {
  background: rgba(245, 158, 11, 0.15);
  color: #d97706;
}
.pill-untested {
  background: var(--surface-2);
  color: var(--ink-soft);
}
.pill-speed {
  background: rgba(37, 99, 235, 0.12);
  color: var(--brand);
}
.pill-geo {
  background: var(--surface-2);
  color: var(--ink-soft);
  font-size: 11px;
}

/* Card Unlock Matrix */
.card-unlock-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.media-chip-mini {
  font-size: 10px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 4px;
}
.media-chip-mini.m-ok {
  background: rgba(15, 138, 95, 0.15);
  color: var(--ok);
}
.media-chip-mini.m-partial {
  background: rgba(245, 158, 11, 0.15);
  color: #d97706;
}
.media-chip-mini.m-blocked {
  background: rgba(220, 38, 38, 0.1);
  color: var(--danger);
  opacity: 0.6;
}
.media-chip-mini.m-none {
  background: var(--surface-2);
  color: var(--ink-soft);
  opacity: 0.5;
}

.card-chain-status {
  font-size: 12px;
  color: var(--brand);
  background: rgba(37, 99, 235, 0.08);
  padding: 4px 8px;
  border-radius: 6px;
}
.chain-source-tag {
  font-size: 11px;
  color: var(--ink-soft);
  margin-left: 4px;
}

/* Card Action Footer */
.card-actions {
  display: flex;
  gap: 6px;
  border-top: 1px solid var(--border);
  padding-top: 10px;
  margin-top: auto;
}
.btn-card-action {
  flex: 1;
  padding: 4px 8px;
  font-size: 12px;
  border-radius: 6px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  cursor: pointer;
}
.btn-card-action:hover {
  background: var(--brand);
  color: #fff;
  border-color: var(--brand);
}

/* Table Card Details */
.table-node-name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}
.node-title-main {
  font-size: 13px;
}
.table-node-subtext {
  font-size: 11px;
  color: var(--ink-soft);
}
.geo-badge {
  font-size: 11px;
  font-weight: 600;
  margin-right: 4px;
}
.geo-ip-text {
  font-size: 11px;
}
.table-media-wrap {
  display: flex;
  gap: 3px;
  flex-wrap: wrap;
}

/* Empty State Card */
.empty-state-card {
  padding: 60px 20px;
  text-align: center;
  background: var(--surface);
  border: 1px dashed var(--border);
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.empty-emoji {
  font-size: 36px;
}

/* Detail Modal */
.detail-modal-card {
  max-width: 800px;
  width: 90%;
  max-height: 85vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.modal-header-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.detail-head-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.node-flag-icon.large {
  font-size: 28px;
}
.modal-close-btn {
  border: none;
  background: transparent;
  font-size: 18px;
  cursor: pointer;
}
.detail-tabs {
  display: flex;
  gap: 6px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 8px;
}
.detail-tab-btn {
  background: transparent;
  border: none;
  padding: 6px 12px;
  font-size: 13px;
  font-weight: 600;
  border-radius: 6px;
  cursor: pointer;
}
.detail-tab-btn.active {
  background: var(--brand);
  color: #fff;
}
.diag-summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}
.diag-card {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.diag-label {
  font-size: 11px;
  color: var(--ink-soft);
}
.diag-val {
  font-size: 16px;
}
.diag-hint {
  font-size: 11px;
  color: var(--ink-soft);
}
.sub-section-title {
  font-size: 14px;
  font-weight: 700;
  margin: 12px 0 8px;
}
.media-detail-table-wrap {
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
}
.media-detail-table {
  width: 100%;
  border-collapse: collapse;
}
.media-detail-table th,
.media-detail-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
}
.platform-name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}
.diag-actions-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 16px;
}
.params-grid {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.param-row {
  display: flex;
  justify-content: space-between;
  padding: 6px 10px;
  background: var(--surface-2);
  border-radius: 6px;
  font-size: 12px;
}
.param-key {
  color: var(--ink-soft);
  font-weight: 600;
}
.code-preview-wrap {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.code-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  font-weight: 600;
}
.code-block {
  background: #0f172a;
  color: #38bdf8;
  padding: 14px;
  border-radius: 8px;
  font-size: 12px;
  max-height: 360px;
  overflow: auto;
}
.spinner-inline {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
.spinner-inline.large {
  width: 28px;
  height: 28px;
  border-width: 3px;
  border-color: rgba(37, 99, 235, 0.2);
  border-top-color: var(--brand);
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
.text-ok {
  color: var(--ok);
}
.text-danger {
  color: var(--danger);
}
.text-warning {
  color: var(--warning);
}
.text-brand {
  color: var(--brand);
}
.text-muted {
  color: var(--ink-soft);
}
.danger-text {
  color: var(--danger);
}
</style>