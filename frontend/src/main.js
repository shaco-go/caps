// CapsLayer 前端入口：渲染设置界面、录制按键，并调用 Go 端绑定的方法。
import './style.css';
import * as App from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { isModifierCode, keyFromEvent, prettyKey } from './keymap.js';

// 前端本地状态，与后端配置/状态保持同步。
const state = {
  settings: { enabled: true, threshold: 200, shortPress: true },
  mappings: [],
  autostart: false,
  running: false,
  recording: null, // 正在录制的目标：{ index, field: 'source' | 'target' }
};

const root = document.getElementById('app');

// 把后端返回的映射补齐为完整的字段结构。
function normalizeMapping(m) {
  return {
    source: m.source || '',
    mode: m.mode || 'key',
    key: m.key || '',
    ctrl: !!m.ctrl,
    alt: !!m.alt,
    shift: !!m.shift,
    win: !!m.win,
    text: m.text || '',
  };
}

// 转义 HTML，避免用户输入被当作标签解析（XSS 防护）。
function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

// 修饰键显示名。
function modLabel(k) {
  return { ctrl: 'Ctrl', alt: 'Alt', shift: 'Shift', win: 'Win' }[k];
}

// 生成目标按键的可读标签，如 "Ctrl+Shift+C"。
function targetLabel(m) {
  if (!m.key) return '';
  const mods = [];
  if (m.mode === 'combo') {
    if (m.ctrl) mods.push('Ctrl');
    if (m.alt) mods.push('Alt');
    if (m.shift) mods.push('Shift');
    if (m.win) mods.push('Win');
  }
  const key = prettyKey(m.key);
  return mods.length ? mods.join('+') + '+' + key : key;
}

// 在底部状态栏显示提示信息（isError 时用红色样式）。
function setStatus(msg, isError = false) {
  const el = document.getElementById('status');
  if (!el) return;
  el.textContent = msg;
  el.className = 'status' + (isError ? ' error' : '');
}

// 应用来自后端的 {enabled, running, autostart} 状态。
function applyState(s) {
  if (!s) return;
  state.settings.enabled = !!s.enabled;
  state.running = !!s.running;
  if (typeof s.autostart === 'boolean') state.autostart = s.autostart;
  syncControls();
}

// 只更新开关、运行状态文字与自启勾选，不重新渲染整个列表。
function syncControls() {
  const enabledEl = document.getElementById('enabled');
  if (enabledEl) enabledEl.checked = state.settings.enabled;
  const autoEl = document.getElementById('autostart');
  if (autoEl) autoEl.checked = state.autostart;
  const el = document.getElementById('running');
  if (el) {
    el.textContent = state.running ? '运行中' : '已暂停';
    el.className = 'toggle-state ' + (state.running ? 'on' : 'off');
  }
}

// 主动向后端拉取一次最新状态。
async function refreshState() {
  try {
    applyState(await App.GetState());
  } catch (_) { /* 忽略偶发失败 */ }
}

// 整体渲染页面骨架。
function render() {
  root.innerHTML = `
    <header>
      <div class="title">
        <span class="logo">⌨</span>
        <div>
          <h1>CapsLayer</h1>
          <p class="sub">按住 Caps Lock 进入改键层，短按保持原始大小写切换</p>
        </div>
      </div>
      <div class="header-right">
        <label class="toggle">
          <input type="checkbox" id="enabled" ${state.settings.enabled ? 'checked' : ''}>
          <span class="track"><span class="thumb"></span></span>
          <span id="running" class="toggle-state"></span>
        </label>
      </div>
    </header>

    <section class="settings">
      <label>长按阈值
        <input type="number" id="threshold" min="50" max="2000" step="10"
               value="${state.settings.threshold}"> ms
      </label>
      <label class="check">
        <input type="checkbox" id="autostart" ${state.autostart ? 'checked' : ''}>
        开机自启
      </label>
    </section>

    <section class="mappings">
      <div class="row table-head">
        <div>源按键（层内）</div>
        <div>方式</div>
        <div>目标</div>
        <div></div>
      </div>
      <div id="rows"></div>
      <button class="add" id="add">+ 添加映射</button>
    </section>

    <footer>
      <span id="status" class="status"></span>
      <button class="primary" id="save">保存映射</button>
      <span class="hint" tabindex="0">?
        <span class="hint-tip">保存内容：长按阈值 + 源按键映射。<br>启用开关与开机自启即时生效，无需保存。</span>
      </span>
    </footer>
  `;
  renderRows();
  bind();
  syncControls();
}

// 渲染单条映射行（源按键、方式选择、目标、删除）。
function rowHtml(m, i) {
  const rec = state.recording;
  const recSource = rec && rec.index === i && rec.field === 'source';
  const recTarget = rec && rec.index === i && rec.field === 'target';

  const sourceLabel = recSource ? '按下按键…' : (prettyKey(m.source) || '录制');

  let target;
  if (m.mode === 'text') {
    target = `<input class="text-input" type="text" data-i="${i}" data-role="text"
               value="${escapeHtml(m.text)}" placeholder="要输出的文本">`;
  } else {
    const mods = m.mode === 'combo'
      ? ['ctrl', 'alt', 'shift', 'win'].map((k) =>
          `<button class="mod ${m[k] ? 'on' : ''}" data-i="${i}" data-role="mod"
                   data-mod="${k}">${modLabel(k)}</button>`).join('')
      : '';
    const label = recTarget ? '按下按键…' : (targetLabel(m) || '录制');
    target = `<div class="combo">${mods}
      <button class="rec ${recTarget ? 'active' : ''}" data-i="${i}"
              data-role="record-target">${escapeHtml(label)}</button></div>`;
  }

  return `
    <div class="row mapping" data-row="${i}">
      <div>
        <button class="rec ${recSource ? 'active' : ''}" data-i="${i}"
                data-role="record-source">${escapeHtml(sourceLabel)}</button>
      </div>
      <div>
        <select data-i="${i}" data-role="mode">
          <option value="key" ${m.mode === 'key' ? 'selected' : ''}>单键</option>
          <option value="combo" ${m.mode === 'combo' ? 'selected' : ''}>组合键</option>
          <option value="text" ${m.mode === 'text' ? 'selected' : ''}>文本</option>
        </select>
      </div>
      <div>${target}</div>
      <div>
        <button class="del" data-i="${i}" data-role="del" title="删除">✕</button>
      </div>
    </div>`;
}

// 重新渲染整个映射列表。
function renderRows() {
  document.getElementById('rows').innerHTML =
    state.mappings.length
      ? state.mappings.map(rowHtml).join('')
      : '<div class="empty">还没有映射，点击下方按钮添加。</div>';
}

// 为顶部开关、阈值、自启以及映射列表绑定事件。
function bind() {
  document.getElementById('enabled').addEventListener('change', async (e) => {
    state.settings.enabled = e.target.checked;
    try {
      await App.SetEnabled(e.target.checked);
      setStatus(e.target.checked ? '已启用' : '已停用');
      refreshState();
    } catch (err) {
      setStatus('切换失败: ' + err, true);
    }
  });

  document.getElementById('threshold').addEventListener('input', (e) => {
    state.settings.threshold = parseInt(e.target.value, 10) || 200;
  });

  document.getElementById('autostart').addEventListener('change', async (e) => {
    state.autostart = e.target.checked;
    try {
      await App.SetAutostart(e.target.checked);
      setStatus(e.target.checked ? '已设置开机自启' : '已取消开机自启');
    } catch (err) {
      setStatus('设置开机自启失败: ' + err, true);
    }
  });

  const rows = document.getElementById('rows');
  rows.addEventListener('click', onRowsClick);
  rows.addEventListener('change', onRowsChange);
  rows.addEventListener('input', onRowsInput);

  document.getElementById('add').addEventListener('click', () => {
    state.mappings.push(normalizeMapping({}));
    renderRows();
  });
  document.getElementById('save').addEventListener('click', save);
}

// 处理映射列表内的点击（录制、修饰键切换、删除）。
function onRowsClick(e) {
  const el = e.target.closest('[data-role]');
  if (!el) return;
  const i = Number(el.dataset.i);
  const role = el.dataset.role;
  if (role === 'record-source') toggleRecording(i, 'source');
  else if (role === 'record-target') toggleRecording(i, 'target');
  else if (role === 'mod') {
    const k = el.dataset.mod;
    state.mappings[i][k] = !state.mappings[i][k];
    renderRows();
  } else if (role === 'del') {
    state.mappings.splice(i, 1);
    renderRows();
  }
}

// 处理“方式”下拉框变化，切换后重绘该行。
function onRowsChange(e) {
  const el = e.target.closest('[data-role="mode"]');
  if (!el) return;
  const m = state.mappings[Number(el.dataset.i)];
  m.mode = el.value;
  renderRows();
}

// 处理文本模式输入框的内容变化。
function onRowsInput(e) {
  const el = e.target.closest('[data-role="text"]');
  if (!el) return;
  state.mappings[Number(el.dataset.i)].text = el.value;
}

// 点击同一录制按钮则取消，否则开始录制。
function toggleRecording(index, field) {
  const rec = state.recording;
  if (rec && rec.index === index && rec.field === field) {
    stopRecording();
    render();
  } else {
    startRecording(index, field);
  }
}

// 进入录制模式：监听全局按键，右键取消。
function startRecording(index, field) {
  stopRecording();
  state.recording = { index, field };
  render();
  setStatus('录制中：按任意键完成，右键或再次点击按钮取消');
  window.addEventListener('keydown', onRecordKey, true);
  window.addEventListener('contextmenu', onRecordCancel, true);
}

// 退出录制模式并移除监听。
function stopRecording() {
  if (!state.recording) return;
  state.recording = null;
  window.removeEventListener('keydown', onRecordKey, true);
  window.removeEventListener('contextmenu', onRecordCancel, true);
  setStatus('');
}

// 右键取消录制。
function onRecordCancel(e) {
  e.preventDefault();
  stopRecording();
  render();
}

// 录制中的按键处理：源按键直接记录，目标按键额外记录组合修饰符。
function onRecordKey(e) {
  if (!state.recording) return;
  e.preventDefault();
  e.stopPropagation();

  const rec = state.recording;
  const m = state.mappings[rec.index];

  if (rec.field === 'source') {
    const k = keyFromEvent(e);
    if (!k) return; // wait for a non-modifier key
    m.source = k;
    stopRecording();
    render();
    return;
  }

  // target
  if (isModifierCode(e.code)) return; // wait for a real key
  const k = keyFromEvent(e);
  if (!k) return;
  m.key = k;
  if (m.mode === 'combo') {
    m.ctrl = e.ctrlKey;
    m.alt = e.altKey;
    m.shift = e.shiftKey;
    m.win = e.metaKey;
  }
  stopRecording();
  render();
}

// 校验并保存配置到后端。
async function save() {
  const thresholdEl = document.getElementById('threshold');
  state.settings.threshold = Math.min(2000, Math.max(50, parseInt(thresholdEl.value, 10) || 200));
  state.settings.shortPress = true;
  const enabledEl = document.getElementById('enabled');
  state.settings.enabled = enabledEl.checked;

  const seen = new Set();
  for (const m of state.mappings) {
    if (!m.source) return setStatus('存在未设置源按键的映射', true);
    if (seen.has(m.source)) return setStatus('源按键重复: ' + prettyKey(m.source), true);
    seen.add(m.source);
    if (m.mode === 'text') {
      if (!m.text) return setStatus('文本映射内容不能为空', true);
    } else if (!m.key) {
      return setStatus('映射 ' + prettyKey(m.source) + ' 缺少目标按键', true);
    }
  }

  try {
    await App.SaveConfig({
      settings: { ...state.settings },
      mappings: state.mappings,
    });
    setStatus('已保存并应用');
    refreshState();
  } catch (err) {
    setStatus('保存失败: ' + err, true);
  }
}

// 初始化：加载配置、首次渲染、订阅状态事件并定时刷新。
async function init() {
  try {
    const cfg = await App.GetConfig();
    state.settings = { ...state.settings, ...(cfg.settings || {}) };
    state.mappings = (cfg.mappings || []).map(normalizeMapping);
    state.autostart = await App.GetAutostart();
    applyState(await App.GetState());
  } catch (err) {
    setStatus('加载配置失败: ' + err, true);
  }
  render();
  EventsOn('engine:state', (s) => applyState(s));
  setInterval(refreshState, 2000);
}

init();
