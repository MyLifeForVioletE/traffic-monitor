function animateNumber(el, target) {
  if (!el) return;
  const current = parseInt(el.textContent) || 0;
  const step = Math.ceil((target - current) / 20);
  const duration = 500;
  const interval = duration / 20;
  let count = current;
  const timer = setInterval(() => {
    count += step;
    if (count >= target) {
      count = target;
      clearInterval(timer);
    }
    el.textContent = count;
  }, interval);
}

function getJSON(url) {
  return fetch(url).then(res => {
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  });
}

function showMainPage() {
  selectPage.style.display = 'none';
  mainPage.style.display = 'block';
  backToSelectBtn.style.display = 'inline-flex';
  
  const selectedIface = availableInterfaces.find(i => i.name === selectedInterface);
  const label = selectedIface ? (selectedIface.description || selectedIface.name) : selectedInterface;
  currentIfacePill.textContent = `📡 ${label}`;
  currentIfacePill.style.display = 'inline-block';
  currentIfaceDesc.textContent = '';
  
  startRefreshTimer();
  setTimeout(refreshCharts, 0);
}

function showSelectPage() {
  stopRefreshTimer();
  selectPage.style.display = 'block';
  mainPage.style.display = 'none';
  backToSelectBtn.style.display = 'none';
  currentIfacePill.style.display = 'none';
  currentIfaceDesc.textContent = '请选择一个网卡并进入监测页面';
  loadConfig();
  loadInterfaces();
}

// 保存配置到本地存储
function saveConfig() {
  captureConfig.topN = parseInt(topNSelect.value) || 20;
  captureConfig.aggregateGranularity = parseInt(aggregateGranularitySelect.value) || 60;

  localStorage.setItem('trafficd_capture_config', JSON.stringify(captureConfig));
}

// 从本地存储加载配置
function loadConfig() {
  const saved = localStorage.getItem('trafficd_capture_config');
  if (saved) {
    try {
      const config = JSON.parse(saved);
      Object.assign(captureConfig, config);

      topNSelect.value = config.topN;
      aggregateGranularitySelect.value = config.aggregateGranularity;
    } catch (e) {
      console.warn('Failed to load config:', e);
    }
  }
  captureConfig.refreshInterval = 1;
  captureConfig.aggregateGranularity = 1;
}

const timeDisplay = document.getElementById("timeDisplay");
const backToSelectBtn = document.getElementById("backToSelectBtn");
const currentIfacePill = document.getElementById("currentIfacePill");
const currentIfaceDesc = document.getElementById("currentIfaceDesc");
const selectPage = document.getElementById("selectPage");
const mainPage = document.getElementById("mainPage");
const topSrc1min = document.getElementById("topSrc1min");
const topSrcAll = document.getElementById("topSrcAll");
const trendCurrentMeta = document.getElementById("trendCurrentMeta");
const trendHistoryMeta = document.getElementById("trendHistoryMeta");
const interfaceGrid = document.getElementById("interfaceGrid");
const startCaptureBtn = document.getElementById("startCaptureBtn");

// 配置表单元素
const topNSelect = document.getElementById("topN");
const aggregateGranularitySelect = document.getElementById("aggregateGranularity");

let selectedInterface = null;

const trendCurrentChart = echarts.init(document.getElementById("trendCurrent"));
const trendHistoryChart = echarts.init(document.getElementById("trendHistory"));

let selectedSrcCurrent = null;
let selectedSrcHistory = null;
let top1minData = [];
let topAllData = [];
let timeStrCurrent = "";
let timeStrHistory = "";
let availableInterfaces = [];

// 配置相关变量
let captureConfig = {
  topN: 20,
  aggregateGranularity: 1,
  refreshInterval: 1
};

function fmt(n) {
  if (n == null) return "-";
  return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

function updateStartButton() {
  const hasInterface = selectedInterface !== null;
  startCaptureBtn.disabled = !hasInterface;
  if (hasInterface) {
    const selectedIface = availableInterfaces.find(i => i.name === selectedInterface);
    const label = selectedIface ? (selectedIface.description || selectedIface.name) : selectedInterface;
    startCaptureBtn.textContent = `开始采集 (${label})`;
  } else {
    startCaptureBtn.textContent = '开始采集';
  }
}



function getIfaceLabel(iface) {
  return iface && iface.description ? iface.description : iface && iface.name ? iface.name : '-';
}

function createInterfaceCard(iface) {
  const card = document.createElement('div');
  card.className = 'interface-card';
  card.dataset.ifaceName = iface.name;

  const isSelected = selectedInterface === iface.name;
  if (isSelected) {
    card.classList.add('selected');
  }

  // 根据接口状态选择不同的图标和颜色
  let icon = '🌐';
  if (iface.wireless) {
    icon = '📶';
  } else if (iface.loopback) {
    icon = '🔄';
  }

  card.innerHTML = `
    <div class="interface-icon">${icon}</div>
    <div class="interface-info">
      <div class="interface-name">${iface.description || iface.name}</div>
      <div class="interface-desc">${iface.name}</div>
    </div>
  `;

  card.addEventListener('click', () => {
    // 移除其他卡片的选中状态
    document.querySelectorAll('.interface-card').forEach(c => c.classList.remove('selected'));
    // 添加当前卡片的选中状态
    card.classList.add('selected');
    selectedInterface = iface.name;
    updateStartButton();
  });

  return card;
}

async function loadInterfaces() {
  try {
    const resp = await getJSON('/api/interfaces');
    availableInterfaces = resp.interfaces || [];
    interfaceGrid.innerHTML = '';

    // 更新统计信息（带动画）
    const totalInterfacesEl = document.getElementById('totalInterfaces');
    const activeInterfacesEl = document.getElementById('activeInterfaces');
    const monitoringSessionsEl = document.getElementById('monitoringSessions');

    animateNumber(totalInterfacesEl, availableInterfaces.length);
    animateNumber(activeInterfacesEl, availableInterfaces.filter(iface => iface.up).length);
    animateNumber(monitoringSessionsEl, resp.current ? 1 : 0);

    // 模拟数据包统计（实际应该从API获取）
    document.getElementById('totalPackets').textContent = '1.2M';

    if (availableInterfaces.length === 0) {
      interfaceGrid.innerHTML = `
        <div class="interface-card" style="text-align: center; cursor: default; border-color: var(--warn);">
          <div class="interface-icon" style="background: linear-gradient(135deg, var(--warn), #f97316);">⚠️</div>
          <div class="interface-name">未检测到网卡</div>
          <div class="interface-desc">请检查网络配置或点击重新检测</div>
        </div>
      `;
      return;
    }

    availableInterfaces.forEach(iface => {
      const card = createInterfaceCard(iface);
      interfaceGrid.appendChild(card);
    });

    // 如果有当前选中的网卡，设置为选中状态
    if (resp.current && availableInterfaces.some(i => i.name === resp.current)) {
      selectedInterface = resp.current;
      const currentCard = document.querySelector(`[data-iface-name="${resp.current}"]`);
      if (currentCard) {
        currentCard.classList.add('selected');
      }
    } else if (availableInterfaces.length > 0) {
      // 默认选择第一个
      selectedInterface = availableInterfaces[0].name;
      const firstCard = document.querySelector(`[data-iface-name="${availableInterfaces[0].name}"]`);
      if (firstCard) {
        firstCard.classList.add('selected');
      }
    }

    updateStartButton();
  } catch (err) {
    interfaceGrid.innerHTML = `
      <div class="interface-card" style="text-align: center; cursor: default;">
        <div class="interface-icon">❌</div>
        <div class="interface-name">加载失败</div>
        <div class="interface-desc">请检查网络连接后重试</div>
      </div>
    `;
    console.warn('加载网卡列表失败:', err.message);
  }
}

async function selectInterface(iface) {
  if (!iface) return false;
  try {
    const res = await fetch(`/api/select-interface?iface=${encodeURIComponent(iface)}`, { method: 'POST' });
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    return true;
  } catch (err) {
    console.error('选择网卡失败:', err.message);
    return false;
  }
}

function refreshCharts() {
  if (trendCurrentChart) trendCurrentChart.resize();
  if (trendHistoryChart) trendHistoryChart.resize();
}

function formatTimeLabel(ts, isMinute = false) {
  if (typeof ts === 'string' && ts.endsWith('s')) {
    return ts;
  }
  const date = new Date(ts);
  if (isMinute) {
    return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit' });
  }
  return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function buildSecondSeries(points) {
  const currentSec = new Date().getSeconds();
  const series = Array.from({ length: 60 }, (_, i) => ({
    sec_ts: `${String(i).padStart(2, '0')}s`,
    flow_size: i <= currentSec ? 0 : null,
    flow_cardinality: i <= currentSec ? 0 : null,
  }));

  (points || []).forEach(p => {
    const date = new Date(p.sec_ts);
    const sec = date.getSeconds();
    if (sec >= 0 && sec < 60 && sec <= currentSec) {
      series[sec] = {
        sec_ts: `${String(sec).padStart(2, '0')}s`,
        flow_size: p.flow_size != null ? p.flow_size : 0,
        flow_cardinality: p.flow_cardinality || p.dst_port_cardinality_est || 0,
      };
    }
  });
  return series;
}

function setTrendOption(chart, points, isMinute = false) {
  const times = points.map(p => formatTimeLabel(p.sec_ts, isMinute));
  const fs = points.map(p => p.flow_size);
  const dp = points.map(p => p.flow_cardinality || p.dst_cardinality_est || 0);
  const option = {
    tooltip: { trigger: 'axis' },
    legend: { top: 0, data: ['流大小', '流基数'] },
    grid: { left: 50, right: 20, top: 30, bottom: 60 },
    xAxis: { type: 'category', data: times, axisLabel: { rotate: 35 } },
    yAxis: [
      { type: 'value', name: '流大小' },
      { type: 'value', name: '流基数', position: 'right' }
    ],
    series: [
      { type: 'line', name: '流大小', data: fs, smooth: true, lineStyle: { color: '#12b3b3' } },
      { type: 'line', name: '流基数', yAxisIndex: 1, data: dp, smooth: true, lineStyle: { color: '#3cc7a6' } }
    ]
  };
  chart.setOption(option, true);
}

function aggregateByMinute(points) {
  const buckets = {};
  points.forEach(p => {
    const date = new Date(p.sec_ts);
    date.setSeconds(0, 0);
    const key = date.toISOString();
    const dstEst = p.dst_port_cardinality_est || 0;
    if (!buckets[key]) {
      buckets[key] = {
        sec_ts: date.toISOString(),
        flow_size: p.flow_size || 0,
        dst_port_cardinality_est: dstEst,
      };
    } else {
      buckets[key].flow_size += p.flow_size || 0;
      if (dstEst > buckets[key].dst_port_cardinality_est) {
        buckets[key].dst_port_cardinality_est = dstEst;
      }
    }
  });
  return Object.values(buckets).sort((a, b) => new Date(a.sec_ts) - new Date(b.sec_ts));
}

function renderTopTable(tableId, data, timeStr, topN) {
  const tbody = document.getElementById(tableId);
  tbody.innerHTML = "";
  const rows = data || [];
  const windowSec = captureConfig.aggregateGranularity || 60;
  const granularityLabel = windowSec >= 300 ? '5 分钟' : (windowSec >= 60 ? '1 分钟' : windowSec + ' 秒');
  const titlePrefix = tableId === 'topSrc1min' ? `当前 ${granularityLabel}` : '历史';
  
  rows.forEach((row, idx) => {
    const tr = document.createElement("tr");
    const isActive = (tableId === 'topSrc1min' ? selectedSrcCurrent : selectedSrcHistory) === row.src_ip;
    if (isActive) tr.classList.add("active");
    const flowSize = row.total_flow_size || row.flow_size || 0;
    const cardEst = row.flow_cardinality || row.dst_cardinality_est || 0;
    const avgPerPort = cardEst > 0 ? (flowSize / cardEst) : 0;
    let status = '正常';
    if (avgPerPort < 1.5 && flowSize > 100) status = '<span style="color:#f59e0b">可疑</span>';
    if (avgPerPort < 1.1 && flowSize > 500) status = '<span style="color:#ef4444">异常</span>';
    tr.innerHTML = `
      <td class="num">${idx + 1}</td>
      <td>${row.src_ip}</td>
      <td>${timeStr}</td>
      <td class="num">${fmt(flowSize)}</td>
      <td class="num">${fmt(cardEst)}</td>
      <td>${status}</td>
    `;
    tr.style.cursor = "pointer";
    tr.addEventListener("click", async () => {
      if (tableId === 'topSrc1min') {
        selectedSrcCurrent = row.src_ip;
        await fetchAndRenderSeries(true, windowSec);
      } else {
        selectedSrcHistory = row.src_ip;
        await fetchAndRenderSeries(false, windowSec);
      }
      renderTopTable('topSrc1min', top1minData, timeStrCurrent, topN);
      renderTopTable('topSrcAll', topAllData, timeStrHistory, topN);
    });
    tbody.appendChild(tr);
  });
}

async function fetchAndRenderSeries(isCurrent, windowSec) {
  if (isCurrent) {
    if (!selectedSrcCurrent) return;
    const granularityLabel = windowSec >= 300 ? '5 分钟' : (windowSec >= 60 ? '1 分钟' : windowSec + ' 秒');
    trendCurrentMeta.textContent = `当前 ${granularityLabel} src: ${selectedSrcCurrent}`;
    const currentSeries = await getJSON(`/api/src-timeseries?src_ip=${encodeURIComponent(selectedSrcCurrent)}&window_sec=${windowSec}`);
    if (windowSec <= 60) {
      setTrendOption(trendCurrentChart, buildSecondSeries(currentSeries.points || []), false);
    } else {
      setTrendOption(trendCurrentChart, aggregateByMinute(currentSeries.points || []), true);
    }
  } else {
    if (!selectedSrcHistory) return;
    trendHistoryMeta.textContent = `历史累计 src: ${selectedSrcHistory}`;
    const historySeries = await getJSON(`/api/src-timeseries?src_ip=${encodeURIComponent(selectedSrcHistory)}&window_sec=3600`);
    setTrendOption(trendHistoryChart, aggregateByMinute(historySeries.points || []), true);
  }
}

function updateSpeedGauge(el, value) {
  const target = parseInt(value) || 0;
  animateNumber(el, target);
}

async function fetchAndRender() {
  const windowSec = captureConfig.aggregateGranularity || 60;
  const topN = captureConfig.topN || 20;
  
  const [latest, top1min, topAll] = await Promise.all([
    getJSON('/api/latest'),
    getJSON(`/api/top-src?window_sec=${windowSec}&top_n=${topN}`),
    getJSON(`/api/top-src-all?top_n=${topN}`),
  ]);
  
  const currentTimeStr = new Date().toLocaleTimeString('zh-CN', { hour12: false });

  // 更新速率仪表盘
  const pps = latest.total_flow_size || 0;
  const avgPacketSize = 500;
  const kbps = Math.round(pps * avgPacketSize * 8 / 1024);
  
  updateSpeedGauge(document.getElementById('speedPps'), pps);
  updateSpeedGauge(document.getElementById('speedBps'), kbps);
  updateSpeedGauge(document.getElementById('speedSrcs'), latest.srcs_unique || 0);
  updateSpeedGauge(document.getElementById('speedFlows'), latest.srcs_unique || 0);

  top1minData = top1min.top || [];
  topAllData = topAll.top || [];
  timeStrCurrent = currentTimeStr;
  timeStrHistory = currentTimeStr;

  if (!selectedSrcCurrent && top1minData.length > 0) {
    selectedSrcCurrent = top1minData[0].src_ip;
  }
  if (!selectedSrcHistory && topAllData.length > 0) {
    selectedSrcHistory = topAllData[0].src_ip;
  }

  renderTopTable("topSrc1min", top1minData, currentTimeStr, topN);
  renderTopTable("topSrcAll", topAllData, currentTimeStr, topN);
  
  const currentEpochTitle = document.getElementById("currentEpochTitle");
  const historyTitle = document.getElementById("historyTitle");
  const granularityLabel = windowSec >= 300 ? '5 分钟' : (windowSec >= 60 ? '1 分钟' : windowSec + ' 秒');
  if (currentEpochTitle) currentEpochTitle.textContent = `当前 ${granularityLabel} TOP ${topN} 流信息`;
  if (historyTitle) historyTitle.textContent = `历史 TOP ${topN} 流信息`;
  
  timeDisplay.textContent = currentTimeStr;
  
  await Promise.all([
    fetchAndRenderSeries(true, windowSec),
    fetchAndRenderSeries(false, windowSec),
  ]);
}

// 保存页面状态到本地存储
function savePageState(isMainPage) {
  localStorage.setItem('trafficd_page_state', JSON.stringify({
    isMainPage,
    selectedInterface
  }));
}

// 从本地存储恢复页面状态
function restorePageState() {
  const saved = localStorage.getItem('trafficd_page_state');
  if (saved) {
    try {
      return JSON.parse(saved);
    } catch (e) {
      console.warn('Failed to restore page state:', e);
    }
  }
  return null;
}

// 初始化页面状态
async function initPageState() {
  await loadInterfaces();
  
  const state = restorePageState();
  
  // 如果之前在监测页面且已选择有效网卡，直接恢复到监测页面
  if (state && state.isMainPage && state.selectedInterface) {
    const ifaceExists = availableInterfaces.some(i => i.name === state.selectedInterface);
    if (ifaceExists) {
      selectedInterface = state.selectedInterface;
      const ok = await selectInterface(selectedInterface);
      if (ok) {
        showMainPage();
        fetchAndRender().catch(err => console.error("加载失败:", err.message));
        return;
      }
    }
  }
  
  // 默认显示选择页面
  showSelectPage();
}

// 开始采集按钮事件
startCaptureBtn.addEventListener("click", async () => {
  if (!selectedInterface) return;

  // 保存配置
  saveConfig();

  // 选择接口并开始监测
  const ok = await selectInterface(selectedInterface);
  if (ok) {
    savePageState(true);
    showMainPage();
    fetchAndRender().catch(err => console.error("加载失败:", err.message));
  }
});

// 返回网卡选择时清除状态
backToSelectBtn.addEventListener("click", () => {
  savePageState(false);
  showSelectPage();
  loadInterfaces().catch(err => {
    console.error("加载网卡列表失败:", err.message);
  });
});

// 初始化页面
initPageState().catch(err => {
  console.warn("初始化页面状态失败:", err.message);
  showSelectPage();
});

// 配置表单变化时自动保存
[topNSelect, aggregateGranularitySelect].forEach(element => {
  element.addEventListener('change', saveConfig);
});

// 动态刷新间隔
let refreshTimer = null;

function startRefreshTimer() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
  }
  refreshTimer = setInterval(() => {
    if (mainPage.style.display === 'block') {
      fetchAndRender().catch(err => {
        console.error("加载失败:", err.message);
      });
    }
  }, captureConfig.refreshInterval * 1000);
}

function stopRefreshTimer() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
}
