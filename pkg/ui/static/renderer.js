const hydratableSelector = '[data-renderer-root], .sidebar-fragment, .transcript';

const PT = window.Pretext || {};
const TEXT_FONT = '500 15px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
const META_FONT = '600 11px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
const TITLE_FONT = '600 14px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
const MONO_FONT = '500 12px "SFMono-Regular", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
const MAX_EVENT_LOG_ITEMS = 48;

const markHydrated = (root) => {
  if (root?.matches?.(hydratableSelector)) {
    root.setAttribute('data-renderer-ready', 'true');
  }

  root?.querySelectorAll?.(hydratableSelector).forEach((node) => {
    node.setAttribute('data-renderer-ready', 'true');
  });
};

const highlight = (root) => {
  if (window.Pretext && typeof window.Pretext.highlight === 'function') {
    window.Pretext.highlight(root || document);
  }
};

const markActiveNavigation = () => {
  const route = document.body?.dataset.route || '';
  document.querySelectorAll('.shell__nav a').forEach((link) => {
    link.classList.toggle('is-active', link.getAttribute('href') === route);
  });
};

const clamp = (value, min, max) => Math.min(max, Math.max(min, value));

const readJSON = (input) => {
  try {
    return JSON.parse(input);
  } catch {
    return null;
  }
};

const formatClock = (timestamp) => {
  if (!timestamp) {
    return '—';
  }
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) {
    return '—';
  }
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
};

const compactWhitespace = (value) => String(value || '').replace(/\s+/g, ' ').trim();

const summarize = (value, maxLength = 140) => {
  const text = compactWhitespace(value);
  if (text.length <= maxLength) {
    return text;
  }
  return `${text.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
};

const roleLabel = (role, kind) => {
  if (role) {
    return role.replace(/_/g, ' ');
  }
  if (kind === 'reasoning') {
    return 'reasoning';
  }
  if (kind === 'tool') {
    return 'tool';
  }
  return 'assistant';
};

const ensurePretextLayout = (text, options) => {
  if (PT && typeof PT.layoutBlock === 'function') {
    return PT.layoutBlock(text, options);
  }

  const content = String(text || '');
  const lines = content.split(/\r?\n/);
  const lineHeight = options?.lineHeight || 22;
  const width = Math.max(...lines.map((line) => (line.length || 1) * 8), 0);
  return { lines, width, height: Math.max(lineHeight, lines.length * lineHeight), lineHeight };
};

const drawRoundedRect = (ctx, x, y, width, height, radius) => {
  const r = Math.min(radius, width / 2, height / 2);
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + width, y, x + width, y + height, r);
  ctx.arcTo(x + width, y + height, x, y + height, r);
  ctx.arcTo(x, y + height, x, y, r);
  ctx.arcTo(x, y, x + width, y, r);
  ctx.closePath();
};

const drawPill = (ctx, x, y, label, fill, stroke, color) => {
  ctx.font = META_FONT;
  const text = String(label || '');
  const metrics = PT.measureText ? PT.measureText(text, { font: META_FONT }) : { width: text.length * 7, height: 14 };
  const width = metrics.width + 20;
  const height = 22;
  drawRoundedRect(ctx, x, y, width, height, 11);
  ctx.fillStyle = fill;
  ctx.fill();
  ctx.strokeStyle = stroke;
  ctx.lineWidth = 1;
  ctx.stroke();
  ctx.fillStyle = color;
  ctx.textBaseline = 'middle';
  ctx.fillText(text.toUpperCase(), x + 10, y + height / 2 + 0.5);
  return width;
};

const measurePillWidth = (label) => {
  const text = String(label || '');
  const metrics = PT.measureText ? PT.measureText(text, { font: META_FONT }) : { width: text.length * 7 };
  return metrics.width + 20;
};

class RunSceneRenderer {
  constructor(root) {
    this.root = root;
    this.canvas = root.querySelector('[data-run-canvas]');
    this.viewport = root.querySelector('[data-run-viewport]') || root.querySelector('.run-scene');
    this.eventLog = root.querySelector('[data-run-event-log]') || root.closest('.panel--main')?.querySelector('[data-run-event-log]') || document.querySelector('[data-run-event-log]');
    this.statusTargets = [
      ...document.querySelectorAll('[data-run-status-badge]'),
      ...document.querySelectorAll('.panel__header--run .status'),
    ];
    this.connectionTarget = root.querySelector('[data-run-connection]');
    this.streamStateTarget = root.querySelector('[data-scene-stream-state]');
    this.stepCountTarget = root.querySelector('[data-scene-step-count]');
    this.entityCountTarget = root.querySelector('[data-scene-entity-count]');
    this.lastEventTarget = root.querySelector('[data-run-last-event]');
    this.emptyState = root.querySelector('[data-run-empty-state]');
    this.runId = root.dataset.runId || '';
    this.eventsUrl = root.dataset.eventsUrl || '';
    this.title = root.dataset.runTitle || this.runId || 'run';
    this.summary = root.dataset.runSummary || '';
    this.scene = {
      runStatus: root.dataset.runStatus || 'starting',
      connection: 'connecting',
      steps: new Map(),
      textBodies: new Map(),
      toolBodies: new Map(),
      flow: [],
      flowIndex: new Map(),
      stepCount: 0,
      lastSeq: 0,
      activeStepName: '',
      activeTextId: '',
      activeReasoningId: '',
      notices: 0,
    };
    this.needsRender = false;
    this.destroyed = false;
    this.dpr = window.devicePixelRatio || 1;
    this.ctx = this.canvas?.getContext?.('2d');
    this.resizeObserver = null;
    this.source = null;

    if (!this.canvas || !this.ctx || !this.eventsUrl) {
      return;
    }

    this.root.dataset.rendererInitialized = 'true';
    this.installResizeObserver();
    this.setConnection('connecting');
    this.updateStatus(this.scene.runStatus);
    this.scheduleRender();
    this.connect();
  }

  installResizeObserver() {
    if (typeof ResizeObserver !== 'function' || !this.viewport) {
      window.addEventListener('resize', () => this.scheduleRender(), { passive: true });
      return;
    }

    this.resizeObserver = new ResizeObserver(() => this.scheduleRender());
    this.resizeObserver.observe(this.viewport);
  }

  connect() {
    if (typeof EventSource !== 'function') {
      this.setConnection('unsupported');
      this.pushNotice('EventSource unsupported', 'Browser does not support Server-Sent Events.');
      return;
    }

    this.source = new EventSource(this.eventsUrl);
    this.source.addEventListener('open', () => {
      this.setConnection('live');
    });
    this.source.addEventListener('message', (event) => {
      this.consumeEnvelope(event);
    });
    this.source.addEventListener('error', () => {
      const readyState = this.source ? this.source.readyState : EventSource.CLOSED;
      if (readyState === EventSource.CLOSED) {
        this.setConnection('closed');
      } else {
        this.setConnection('reconnecting');
      }
    });
  }

  consumeEnvelope(event) {
    const payload = readJSON(event.data);
    if (!payload) {
      return;
    }

    const envelope = this.normalizeEnvelope(payload, event.lastEventId);
    if (!envelope) {
      return;
    }

    const seq = Number(envelope.sequence || 0);
    if (Number.isFinite(seq) && seq > 0) {
      this.scene.lastSeq = seq;
    }

    if (envelope.type === 'session.snapshot' && envelope.data) {
      this.applySnapshot(envelope, seq);
      this.appendEventLog('session.snapshot', `snapshot seq ${envelope.data.snapshot_sequence || seq || '—'}`);
      this.scheduleRender();
      return;
    }

    const aguiEvent = this.extractAGUIEvent(envelope);
    if (!aguiEvent) {
      this.appendEventLog(envelope.type || 'unknown', this.describeEvent(envelope));
      this.scheduleRender();
      return;
    }

    this.applyAGUIEvent(aguiEvent, seq);
    this.appendEventLog(aguiEvent.type || envelope.type || 'unknown', this.describeEvent(aguiEvent));
    this.scheduleRender();
  }

  normalizeEnvelope(payload, fallbackLastEventId) {
    if (!payload || typeof payload !== 'object') {
      return null;
    }

    const sequence = Number(payload.sequence || fallbackLastEventId || 0);
    if (payload.type === 'session.snapshot') {
      return {
        ...payload,
        sequence,
        data: payload.data && typeof payload.data === 'object' ? payload.data : readJSON(payload.data || 'null'),
      };
    }

    if (payload.session_id && Object.prototype.hasOwnProperty.call(payload, 'data')) {
      return {
        ...payload,
        sequence,
        data: payload.data && typeof payload.data === 'object' ? payload.data : readJSON(payload.data || 'null'),
      };
    }

    return {
      type: payload.type || 'agui.raw',
      sequence,
      raw: payload,
      data: payload,
    };
  }

  extractAGUIEvent(envelope) {
    if (!envelope) {
      return null;
    }
    if (envelope.raw && envelope.raw.type && envelope.raw.type !== 'session.snapshot') {
      return envelope.raw;
    }
    if (envelope.data && typeof envelope.data === 'object' && envelope.data.type) {
      return envelope.data;
    }
    return null;
  }

  applySnapshot(event, seq) {
    const snapshot = event.data || {};
    if (snapshot.run_id) {
      this.runId = snapshot.run_id;
    }
    if (snapshot.status) {
      this.updateStatus(snapshot.status);
    }

    const approvals = snapshot.pending_approvals && typeof snapshot.pending_approvals === 'object' ? snapshot.pending_approvals : {};
    Object.values(approvals).forEach((approval, index) => {
      if (!approval || typeof approval !== 'object') {
        return;
      }
      const toolId = approval.ToolCallID || approval.tool_call_id || approval.toolCallId || `approval_${index}`;
      const toolName = approval.ToolName || approval.tool_name || approval.toolName || 'approval';
      const args = approval.ArgsJSON || approval.args_json || approval.argsJson || '';
      const tool = this.ensureToolBody(toolId, toolName, Date.now());
      if (tool) {
        tool.status = 'approval';
        tool.args = String(args || tool.args || '');
      }
    });

    if (snapshot.waiting_reason && !Object.keys(approvals).length) {
      this.pushNotice('Run waiting', snapshot.waiting_reason, Date.now());
    }

    this.setLastEventMeta(seq, 'snapshot');
  }

  applyAGUIEvent(event, seq) {
    const timestamp = event.timestamp || Date.now();
    switch (event.type) {
      case 'RUN_STARTED':
        this.runId = event.runId || this.runId;
        this.updateStatus('running');
        this.pushNotice('Run started', this.runId || 'live session', timestamp);
        break;
      case 'RUN_FINISHED':
        if (this.scene.runStatus !== 'waiting') {
          this.updateStatus('completed');
        }
        this.pushNotice('Run finished', this.runId || 'completed', timestamp);
        break;
      case 'RUN_ERROR':
        this.updateStatus('failed');
        this.pushNotice('Run error', event.message || 'Unknown failure', timestamp);
        break;
      case 'STEP_STARTED':
        this.scene.activeStepName = event.stepName || this.scene.activeStepName;
        this.ensureStep(event.stepName || `step_${this.scene.stepCount + 1}`, timestamp, 'running');
        break;
      case 'STEP_FINISHED':
        this.finishStep(event.stepName || this.scene.activeStepName, timestamp);
        if ((event.stepName || '') === this.scene.activeStepName) {
          this.scene.activeStepName = '';
        }
        break;
      case 'TEXT_MESSAGE_START':
        this.scene.activeTextId = event.messageId;
        this.ensureTextBody(event.messageId, 'text', event.role || 'assistant', timestamp);
        break;
      case 'TEXT_MESSAGE_CONTENT':
        this.appendToTextBody(event.messageId, 'text', event.delta || '', timestamp);
        break;
      case 'TEXT_MESSAGE_END':
        this.finishTextBody(event.messageId, 'text', timestamp);
        if (this.scene.activeTextId === event.messageId) {
          this.scene.activeTextId = '';
        }
        break;
      case 'REASONING_START':
      case 'REASONING_MESSAGE_START':
        this.scene.activeReasoningId = event.messageId;
        this.ensureTextBody(event.messageId, 'reasoning', event.role || 'reasoning', timestamp);
        break;
      case 'REASONING_MESSAGE_CONTENT':
        this.appendToTextBody(event.messageId, 'reasoning', event.delta || '', timestamp);
        break;
      case 'REASONING_MESSAGE_END':
      case 'REASONING_END':
        this.finishTextBody(event.messageId, 'reasoning', timestamp);
        if (this.scene.activeReasoningId === event.messageId) {
          this.scene.activeReasoningId = '';
        }
        break;
      case 'TOOL_CALL_START':
        this.ensureToolBody(event.toolCallId, event.toolCallName || 'tool', timestamp);
        break;
      case 'TOOL_CALL_ARGS':
        this.appendToolArgs(event.toolCallId, event.delta || '', timestamp);
        break;
      case 'TOOL_CALL_END':
        this.finishToolBody(event.toolCallId, timestamp);
        break;
      case 'TOOL_CALL_RESULT':
        this.attachToolResult(event.toolCallId, event.content || '', event.role || 'tool', timestamp);
        break;
      case 'CUSTOM':
        this.applyCustomEvent(event.name, event.value, timestamp);
        break;
      default:
        break;
    }

    this.setLastEventMeta(seq, event.type || 'event');
  }

  applyCustomEvent(name, value, timestamp) {
    const payload = value && typeof value === 'object' ? value : readJSON(JSON.stringify(value || {})) || {};
    switch (name) {
      case 'gollem.run.waiting':
        this.updateStatus('waiting');
        this.pushNotice('Run waiting', payload.reason || 'paused', timestamp);
        break;
      case 'gollem.run.resumed':
        this.updateStatus('running');
        this.pushNotice('Run resumed', payload.runId || this.runId || 'stream resumed', timestamp);
        break;
      case 'gollem.approval.requested': {
        const tool = this.ensureToolBody(payload.toolCallId || `approval_${Date.now()}`, payload.toolName || 'approval', timestamp);
        if (tool) {
          tool.status = 'approval';
          tool.args = compactWhitespace(payload.argsJson || tool.args || '');
        }
        break;
      }
      case 'gollem.approval.resolved': {
        const tool = this.scene.toolBodies.get(payload.toolCallId || '');
        if (tool) {
          tool.status = payload.approved ? 'approved' : 'denied';
          tool.updatedAt = timestamp;
        }
        break;
      }
      case 'gollem.deferred.requested':
        this.pushNotice('Deferred input', payload.toolName || payload.toolCallId || 'awaiting input', timestamp);
        break;
      case 'gollem.deferred.resolved':
        this.pushNotice('Deferred resolved', summarize(payload.content || payload.toolName || ''), timestamp);
        break;
      default:
        break;
    }
  }

  describeEvent(event) {
    switch (event.type) {
      case 'RUN_STARTED':
        return event.runId || 'run started';
      case 'RUN_FINISHED':
        return event.runId || 'run finished';
      case 'RUN_ERROR':
        return summarize(event.message || 'run error');
      case 'STEP_STARTED':
      case 'STEP_FINISHED':
        return event.stepName || 'step';
      case 'TEXT_MESSAGE_CONTENT':
      case 'REASONING_MESSAGE_CONTENT':
        return summarize(event.delta || '', 120);
      case 'TEXT_MESSAGE_START':
      case 'TEXT_MESSAGE_END':
      case 'REASONING_START':
      case 'REASONING_END':
        return event.messageId || 'message';
      case 'TOOL_CALL_START':
        return `${event.toolCallName || 'tool'} · ${event.toolCallId || ''}`.trim();
      case 'TOOL_CALL_ARGS':
        return summarize(event.delta || '', 120);
      case 'TOOL_CALL_RESULT':
        return summarize(event.content || '', 120);
      case 'CUSTOM':
        return event.name || 'custom';
      default:
        return summarize(JSON.stringify(event), 120);
    }
  }

  setLastEventMeta(seq, label) {
    if (!this.lastEventTarget) {
      return;
    }
    const seqLabel = seq ? `#${seq}` : 'live';
    this.lastEventTarget.textContent = `${seqLabel} · ${label}`;
  }

  updateStatus(status) {
    this.scene.runStatus = status || this.scene.runStatus || 'running';
    this.statusTargets.forEach((target) => {
      if (!target) {
        return;
      }
      target.textContent = this.scene.runStatus;
      const baseClasses = Array.from(target.classList).filter((name) => !name.startsWith('status--'));
      target.className = baseClasses.concat(`status--${this.scene.runStatus}`).join(' ');
    });
  }

  setConnection(connection) {
    this.scene.connection = connection;
    if (this.connectionTarget) {
      this.connectionTarget.textContent = `SSE ${connection}`;
    }
    if (this.streamStateTarget) {
      this.streamStateTarget.textContent = connection;
    }
    this.scheduleRender();
  }

  pushFlow(item) {
    if (this.scene.flowIndex.has(item.id)) {
      return this.scene.flow[this.scene.flowIndex.get(item.id)];
    }
    const record = { ...item, order: this.scene.flow.length + 1 };
    this.scene.flowIndex.set(record.id, this.scene.flow.length);
    this.scene.flow.push(record);
    return record;
  }

  pushNotice(title, detail, timestamp = Date.now()) {
    this.scene.notices += 1;
    return this.pushFlow({
      id: `notice:${this.scene.notices}`,
      kind: 'notice',
      title,
      detail,
      content: detail,
      createdAt: timestamp,
      updatedAt: timestamp,
      stepName: this.scene.activeStepName,
      status: 'info',
    });
  }

  ensureStep(stepName, timestamp, status) {
    if (!stepName) {
      return null;
    }
    let step = this.scene.steps.get(stepName);
    if (!step) {
      this.scene.stepCount += 1;
      step = this.pushFlow({
        id: `step:${stepName}`,
        kind: 'step',
        title: stepName.replace(/_/g, ' '),
        stepName,
        createdAt: timestamp,
        updatedAt: timestamp,
        status: status || 'running',
        index: this.scene.stepCount,
      });
      this.scene.steps.set(stepName, step);
    }
    step.status = status || step.status || 'running';
    step.updatedAt = timestamp;
    return step;
  }

  finishStep(stepName, timestamp) {
    const step = this.scene.steps.get(stepName || '');
    if (!step) {
      return;
    }
    step.status = 'completed';
    step.updatedAt = timestamp;
  }

  ensureTextBody(messageId, kind, role, timestamp) {
    if (!messageId) {
      return null;
    }
    let body = this.scene.textBodies.get(messageId);
    if (!body) {
      body = this.pushFlow({
        id: `${kind}:${messageId}`,
        kind,
        messageId,
        role: roleLabel(role, kind),
        title: kind === 'reasoning' ? 'Reasoning' : 'Text stream',
        content: '',
        createdAt: timestamp,
        updatedAt: timestamp,
        status: 'streaming',
        stepName: this.scene.activeStepName,
      });
      this.scene.textBodies.set(messageId, body);
    }
    body.updatedAt = timestamp;
    if (role) {
      body.role = roleLabel(role, kind);
    }
    if (!body.stepName && this.scene.activeStepName) {
      body.stepName = this.scene.activeStepName;
    }
    return body;
  }

  appendToTextBody(messageId, kind, delta, timestamp) {
    const body = this.ensureTextBody(messageId, kind, undefined, timestamp);
    if (!body) {
      return;
    }
    body.content += delta || '';
    body.updatedAt = timestamp;
    body.status = 'streaming';
  }

  finishTextBody(messageId, kind, timestamp) {
    const body = this.scene.textBodies.get(messageId || '');
    if (!body || body.kind !== kind) {
      return;
    }
    body.updatedAt = timestamp;
    body.status = 'complete';
  }

  ensureToolBody(toolCallId, toolCallName, timestamp) {
    if (!toolCallId) {
      return null;
    }
    let tool = this.scene.toolBodies.get(toolCallId);
    if (!tool) {
      tool = this.pushFlow({
        id: `tool:${toolCallId}`,
        kind: 'tool',
        toolCallId,
        title: toolCallName || 'tool',
        args: '',
        result: '',
        role: 'tool',
        createdAt: timestamp,
        updatedAt: timestamp,
        status: 'pending',
        stepName: this.scene.activeStepName,
      });
      this.scene.toolBodies.set(toolCallId, tool);
    }
    if (toolCallName) {
      tool.title = toolCallName;
    }
    tool.updatedAt = timestamp;
    if (!tool.stepName && this.scene.activeStepName) {
      tool.stepName = this.scene.activeStepName;
    }
    return tool;
  }

  appendToolArgs(toolCallId, delta, timestamp) {
    const tool = this.ensureToolBody(toolCallId, undefined, timestamp);
    if (!tool) {
      return;
    }
    tool.args += delta || '';
    tool.updatedAt = timestamp;
    tool.status = tool.status === 'approval' ? 'approval' : 'running';
  }

  finishToolBody(toolCallId, timestamp) {
    const tool = this.scene.toolBodies.get(toolCallId || '');
    if (!tool) {
      return;
    }
    tool.updatedAt = timestamp;
    if (!tool.result) {
      tool.status = tool.status === 'approval' ? 'approval' : 'called';
    }
  }

  attachToolResult(toolCallId, content, role, timestamp) {
    const tool = this.ensureToolBody(toolCallId, role === 'tool' ? undefined : role, timestamp);
    if (!tool) {
      return;
    }
    tool.result = String(content || '');
    tool.updatedAt = timestamp;
    tool.status = /^error:/i.test(tool.result) ? 'failed' : 'returned';
  }

  updateCounters() {
    if (this.stepCountTarget) {
      this.stepCountTarget.textContent = String(this.scene.steps.size);
    }
    if (this.entityCountTarget) {
      this.entityCountTarget.textContent = String(this.scene.flow.length);
    }
    if (this.emptyState) {
      this.emptyState.hidden = this.scene.flow.length > 0;
    }
  }

  appendEventLog(type, detail) {
    if (!this.eventLog) {
      return;
    }

    const item = document.createElement('li');
    const strong = document.createElement('strong');
    strong.textContent = type;
    item.appendChild(strong);

    if (detail) {
      const span = document.createElement('span');
      span.textContent = ` ${detail}`;
      item.appendChild(span);
    }

    this.eventLog.appendChild(item);
    while (this.eventLog.children.length > MAX_EVENT_LOG_ITEMS) {
      this.eventLog.removeChild(this.eventLog.firstElementChild);
    }
    this.eventLog.scrollTop = this.eventLog.scrollHeight;
  }

  scheduleRender() {
    if (this.destroyed || this.needsRender || !this.ctx) {
      this.updateCounters();
      return;
    }
    this.needsRender = true;
    this.updateCounters();
    window.requestAnimationFrame(() => {
      this.needsRender = false;
      this.render();
    });
  }

  resizeCanvas(cssWidth, cssHeight) {
    const dpr = window.devicePixelRatio || 1;
    if (this.dpr !== dpr) {
      this.dpr = dpr;
    }
    const pixelWidth = Math.max(1, Math.round(cssWidth * this.dpr));
    const pixelHeight = Math.max(1, Math.round(cssHeight * this.dpr));
    if (this.canvas.width !== pixelWidth || this.canvas.height !== pixelHeight) {
      this.canvas.width = pixelWidth;
      this.canvas.height = pixelHeight;
      this.canvas.style.width = `${cssWidth}px`;
      this.canvas.style.height = `${cssHeight}px`;
    }
    this.ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
  }

  computeLayout(width) {
    const compactHeader = width < 640;
    const headerHeight = compactHeader ? 84 : 52;
    const pad = clamp(Math.round(width * 0.032), 16, 28);
    const gap = width < 720 ? 14 : 24;
    const contentWidth = Math.max(240, width - pad * 2);
    const singleColumn = width < 860;
    const desiredRail = clamp(Math.round(contentWidth * (singleColumn ? 0.34 : 0.28)), 150, 320);
    const minimumMain = singleColumn ? 240 : 320;
    let railWidth = singleColumn ? Math.min(desiredRail, Math.max(140, Math.round(contentWidth * 0.42))) : desiredRail;
    let mainWidth = singleColumn ? contentWidth : contentWidth - railWidth - gap;

    if (mainWidth < minimumMain) {
      if (singleColumn) {
        railWidth = Math.min(railWidth, Math.max(120, contentWidth - 56));
        mainWidth = contentWidth;
      } else {
        const overflow = minimumMain - mainWidth;
        railWidth = Math.max(150, railWidth - overflow);
        mainWidth = contentWidth - railWidth - gap;
        if (mainWidth < minimumMain || railWidth < 170) {
          railWidth = Math.min(Math.max(140, Math.round(contentWidth * 0.38)), contentWidth - 40);
          mainWidth = contentWidth;
        }
      }
    }

    const stackTools = singleColumn || mainWidth >= contentWidth - 8;
    mainWidth = Math.max(240, Math.min(contentWidth, mainWidth));
    railWidth = Math.max(120, Math.min(contentWidth, railWidth));

    let cursorY = 16 + headerHeight + 18;
    let lastNarrativeY = cursorY;

    const items = this.scene.flow.map((item) => ({ ...item }));
    items.forEach((item) => {
      if (item.kind === 'step') {
        item.x = pad;
        item.y = cursorY;
        item.width = contentWidth;
        item.height = 38;
        cursorY += item.height + 18;
        lastNarrativeY = item.y + item.height / 2;
        return;
      }

      if (item.kind === 'tool') {
        const preview = this.buildToolPreview(item);
        const previewWidth = Math.max(120, (stackTools ? Math.min(contentWidth, railWidth) : railWidth) - 42);
        item.previewLayout = ensurePretextLayout(preview, {
          maxWidth: previewWidth,
          font: MONO_FONT,
          lineHeight: 18,
        });
        item.width = stackTools ? Math.min(contentWidth, Math.max(220, Math.min(contentWidth, railWidth + 28))) : railWidth;
        item.x = stackTools ? pad : pad + mainWidth + gap;
        item.y = cursorY;
        item.height = 78 + item.previewLayout.height;
        item.linkY = lastNarrativeY;
        cursorY += item.height + (stackTools ? 18 : 14);
        return;
      }

      const inset = item.kind === 'reasoning' ? 28 : item.kind === 'notice' ? 12 : 0;
      const bodyWidth = Math.max(220, mainWidth - inset);
      const content = item.kind === 'notice' ? (item.detail || item.content || item.title) : (item.content || '…');
      item.layout = ensurePretextLayout(content, {
        maxWidth: Math.max(140, bodyWidth - 32),
        font: item.kind === 'reasoning' ? MONO_FONT : TEXT_FONT,
        lineHeight: item.kind === 'reasoning' ? 20 : 22,
      });
      item.x = pad + inset;
      item.y = cursorY;
      item.width = bodyWidth;
      item.height = 70 + item.layout.height;
      cursorY += item.height + 18;
      lastNarrativeY = item.y + item.height / 2;
    });

    return {
      items,
      height: Math.max(400, cursorY + 28),
      mainWidth,
      railWidth,
      pad,
      gap,
      stackTools,
    };
  }

  buildToolPreview(item) {
    const parts = [];
    if (item.args) {
      parts.push(`args ${summarize(item.args, 220)}`);
    }
    if (item.result) {
      parts.push(`result ${summarize(item.result, 220)}`);
    }
    if (!parts.length) {
      parts.push(item.status || 'pending');
    }
    return parts.join('\n');
  }

  renderBackground(width, height) {
    const gradient = this.ctx.createLinearGradient(0, 0, width, height);
    gradient.addColorStop(0, '#020617');
    gradient.addColorStop(0.55, '#0b1120');
    gradient.addColorStop(1, '#020617');
    this.ctx.fillStyle = gradient;
    this.ctx.fillRect(0, 0, width, height);

    const glow = this.ctx.createRadialGradient(width * 0.18, 32, 18, width * 0.18, 32, width * 0.6);
    glow.addColorStop(0, 'rgba(56, 189, 248, 0.18)');
    glow.addColorStop(1, 'rgba(56, 189, 248, 0)');
    this.ctx.fillStyle = glow;
    this.ctx.fillRect(0, 0, width, height);

    this.ctx.strokeStyle = 'rgba(96, 165, 250, 0.08)';
    this.ctx.lineWidth = 1;
    for (let x = 0; x < width; x += 32) {
      this.ctx.beginPath();
      this.ctx.moveTo(x + 0.5, 0);
      this.ctx.lineTo(x + 0.5, height);
      this.ctx.stroke();
    }
    for (let y = 0; y < height; y += 32) {
      this.ctx.beginPath();
      this.ctx.moveTo(0, y + 0.5);
      this.ctx.lineTo(width, y + 0.5);
      this.ctx.stroke();
    }
  }

  renderHeader(width) {
    const compactHeader = width < 640;
    const headerHeight = compactHeader ? 84 : 52;
    this.ctx.fillStyle = 'rgba(2, 6, 23, 0.72)';
    drawRoundedRect(this.ctx, 20, 16, width - 40, headerHeight, 18);
    this.ctx.fill();
    this.ctx.strokeStyle = 'rgba(125, 211, 252, 0.18)';
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    this.ctx.font = TITLE_FONT;
    this.ctx.fillStyle = '#e2e8f0';
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(this.title, 34, 35);

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = '#94a3b8';
    this.ctx.fillText(summarize(this.summary || this.runId || 'Live AG-UI stream', compactHeader ? 56 : 48), 34, 53);

    if (compactHeader) {
      const statusWidth = drawPill(this.ctx, 34, 68, this.scene.runStatus, 'rgba(15, 23, 42, 0.96)', 'rgba(125, 211, 252, 0.24)', '#7dd3fc');
      drawPill(this.ctx, 34 + statusWidth + 10, 68, this.scene.connection, 'rgba(15, 23, 42, 0.96)', 'rgba(148, 163, 184, 0.24)', '#cbd5e1');
      return;
    }

    const statusWidth = measurePillWidth(this.scene.runStatus);
    const connectionWidth = measurePillWidth(this.scene.connection);
    const totalWidth = statusWidth + connectionWidth + 10;
    const statusX = Math.max(34, width - 34 - totalWidth);
    const drawnStatusWidth = drawPill(this.ctx, statusX, 28, this.scene.runStatus, 'rgba(15, 23, 42, 0.96)', 'rgba(125, 211, 252, 0.24)', '#7dd3fc');
    drawPill(this.ctx, statusX + drawnStatusWidth + 10, 28, this.scene.connection, 'rgba(15, 23, 42, 0.96)', 'rgba(148, 163, 184, 0.24)', '#cbd5e1');
  }

  renderStep(item) {
    drawRoundedRect(this.ctx, item.x, item.y, item.width, item.height, 16);
    this.ctx.fillStyle = 'rgba(15, 23, 42, 0.82)';
    this.ctx.fill();
    this.ctx.strokeStyle = item.status === 'completed' ? 'rgba(52, 211, 153, 0.32)' : 'rgba(125, 211, 252, 0.24)';
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    this.ctx.fillStyle = item.status === 'completed' ? '#86efac' : '#7dd3fc';
    this.ctx.font = META_FONT;
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(`STEP ${item.index}`, item.x + 16, item.y + item.height / 2 + 0.5);

    this.ctx.font = TITLE_FONT;
    this.ctx.fillStyle = '#e2e8f0';
    this.ctx.fillText(item.title, item.x + 78, item.y + item.height / 2 + 0.5);

    this.ctx.fillStyle = '#64748b';
    this.ctx.fillText(formatClock(item.updatedAt), item.x + item.width - 74, item.y + item.height / 2 + 0.5);
  }

  renderTextCard(item) {
    const accent = item.kind === 'reasoning' ? '#c084fc' : item.kind === 'notice' ? '#fbbf24' : '#7dd3fc';
    const border = item.kind === 'reasoning' ? 'rgba(192, 132, 252, 0.28)' : item.kind === 'notice' ? 'rgba(251, 191, 36, 0.28)' : 'rgba(125, 211, 252, 0.24)';
    const fill = item.kind === 'reasoning' ? 'rgba(36, 18, 59, 0.62)' : item.kind === 'notice' ? 'rgba(56, 44, 18, 0.52)' : 'rgba(15, 23, 42, 0.86)';

    drawRoundedRect(this.ctx, item.x, item.y, item.width, item.height, 20);
    this.ctx.fillStyle = fill;
    this.ctx.fill();
    this.ctx.strokeStyle = border;
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    this.ctx.fillStyle = accent;
    this.ctx.fillRect(item.x + 16, item.y + 16, 4, item.height - 32);

    drawPill(this.ctx, item.x + 32, item.y + 16, roleLabel(item.role, item.kind), 'rgba(2, 6, 23, 0.5)', border, accent);

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = '#94a3b8';
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(formatClock(item.updatedAt), item.x + item.width - 74, item.y + 26.5);

    this.ctx.font = item.kind === 'reasoning' ? MONO_FONT : TEXT_FONT;
    this.ctx.fillStyle = item.kind === 'notice' ? '#fde68a' : '#e2e8f0';
    this.ctx.textBaseline = 'top';
    const startX = item.x + 32;
    const startY = item.y + 48;
    item.layout.lines.forEach((line, index) => {
      this.ctx.fillText(line || ' ', startX, startY + index * item.layout.lineHeight);
    });
  }

  renderTool(item) {
    const stroke = item.status === 'failed' ? 'rgba(248, 113, 113, 0.36)' : item.status === 'returned' ? 'rgba(52, 211, 153, 0.34)' : 'rgba(125, 211, 252, 0.26)';
    const fill = item.status === 'failed' ? 'rgba(69, 10, 10, 0.72)' : 'rgba(15, 23, 42, 0.92)';
    const markerColor = item.status === 'failed' ? '#f87171' : item.status === 'returned' ? '#34d399' : item.status === 'approval' ? '#fbbf24' : '#7dd3fc';
    const markerX = item.x + 14;
    const markerY = item.y + 20;

    this.ctx.strokeStyle = 'rgba(148, 163, 184, 0.18)';
    this.ctx.lineWidth = 1.5;
    this.ctx.beginPath();
    this.ctx.moveTo(item.x - 18, item.linkY || item.y);
    this.ctx.lineTo(markerX, markerY);
    this.ctx.stroke();

    this.ctx.beginPath();
    this.ctx.arc(markerX, markerY, 6, 0, Math.PI * 2);
    this.ctx.fillStyle = markerColor;
    this.ctx.fill();

    drawRoundedRect(this.ctx, item.x + 28, item.y, item.width - 28, item.height, 18);
    this.ctx.fillStyle = fill;
    this.ctx.fill();
    this.ctx.strokeStyle = stroke;
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    drawPill(this.ctx, item.x + 42, item.y + 14, item.title || 'tool', 'rgba(2, 6, 23, 0.5)', stroke, markerColor);

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = '#94a3b8';
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(item.toolCallId || '', item.x + 42, item.y + 42.5);

    this.ctx.font = MONO_FONT;
    this.ctx.fillStyle = '#dbeafe';
    this.ctx.textBaseline = 'top';
    const startX = item.x + 42;
    const startY = item.y + 56;
    item.previewLayout.lines.forEach((line, index) => {
      this.ctx.fillText(line || ' ', startX, startY + index * item.previewLayout.lineHeight);
    });
  }

  render() {
    if (!this.ctx || !this.viewport) {
      return;
    }

    const bounds = this.viewport.getBoundingClientRect();
    const width = Math.max(320, Math.round(bounds.width || this.canvas.clientWidth || 320));
    const layout = this.computeLayout(width);
    this.resizeCanvas(width, layout.height);
    this.renderBackground(width, layout.height);
    this.renderHeader(width);

    layout.items.forEach((item) => {
      if (item.kind === 'step') {
        this.renderStep(item);
        return;
      }
      if (item.kind === 'tool') {
        this.renderTool(item);
        return;
      }
      this.renderTextCard(item);
    });
  }

  destroy() {
    this.destroyed = true;
    if (this.source) {
      this.source.close();
      this.source = null;
    }
    if (this.resizeObserver) {
      this.resizeObserver.disconnect();
      this.resizeObserver = null;
    }
  }
}

const runScenes = new WeakMap();

const initRunScenes = (root = document) => {
  const nodes = [];
  if (root?.matches?.('[data-run-scene]')) {
    nodes.push(root);
  }
  root?.querySelectorAll?.('[data-run-scene]').forEach((node) => nodes.push(node));

  nodes.forEach((node) => {
    if (runScenes.has(node)) {
      return;
    }
    runScenes.set(node, new RunSceneRenderer(node));
  });
};

const hydrate = (root = document) => {
  markHydrated(root);
  highlight(root);
  markActiveNavigation();
  initRunScenes(root);
};

document.addEventListener('DOMContentLoaded', () => {
  hydrate(document);
});

document.body?.addEventListener('htmx:load', (event) => {
  hydrate(event.target || document);
});

document.body?.addEventListener('ui:fragment-loaded', (event) => {
  hydrate(event.target || document);
});
