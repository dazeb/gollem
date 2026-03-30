const hydratableSelector = '[data-renderer-root], .sidebar-fragment, .transcript';

const PT = window.Pretext || {};
const TEXT_FONT = '500 15px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
const META_FONT = '600 11px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
const TITLE_FONT = '600 14px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
const MONO_FONT = '500 12px "SFMono-Regular", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
const TOOL_TITLE_FONT = '600 12px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
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
const lerp = (from, to, amount) => from + (to - from) * amount;
const smoothAmount = (rate, dt) => clamp(1 - Math.exp(-rate * dt), 0, 1);
const motionSettled = (current, target, epsilon = 0.6) => Math.abs((current || 0) - (target || 0)) <= epsilon;
const sceneClock = () => (window.performance && typeof window.performance.now === 'function' ? window.performance.now() : Date.now());

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

const measureTextWidth = (text, font) => {
  const value = String(text || '');
  if (PT && typeof PT.measureText === 'function') {
    return PT.measureText(value, { font }).width || 0;
  }
  const factor = font === MONO_FONT ? 7.1 : font === META_FONT ? 6.8 : font === TOOL_TITLE_FONT ? 7.2 : 8.2;
  return value.length * factor;
};

const ensurePretextLayout = (text, options) => {
  if (PT && typeof PT.layoutBlock === 'function') {
    return PT.layoutBlock(text, options);
  }

  const content = String(text || '');
  const font = options?.font || TEXT_FONT;
  const lineHeight = options?.lineHeight || 22;
  const maxWidth = Math.max(80, options?.maxWidth || 240);
  const paragraphs = content.split(/\r?\n/);
  const lines = [];

  paragraphs.forEach((paragraph, index) => {
    const textLine = paragraph.trimEnd();
    if (!textLine) {
      lines.push('');
      return;
    }

    const tokens = textLine.match(/\S+\s*/g) || [textLine];
    let current = '';
    tokens.forEach((token) => {
      const candidate = `${current}${token}`;
      if (!current || measureTextWidth(candidate, font) <= maxWidth) {
        current = candidate;
        return;
      }
      lines.push(current.trimEnd());
      current = token;
    });
    if (current || index === paragraphs.length - 1) {
      lines.push(current.trimEnd());
    }
  });

  const width = Math.min(maxWidth, Math.max(...lines.map((line) => measureTextWidth(line || ' ', font)), 0));
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
  const metrics = PT.measureText ? PT.measureText(text, { font: META_FONT }) : { width: measureTextWidth(text, META_FONT), height: 14 };
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
  const metrics = PT.measureText ? PT.measureText(text, { font: META_FONT }) : { width: measureTextWidth(text, META_FONT) };
  return metrics.width + 20;
};

const takeFittingPrefix = (value, maxWidth, font) => {
  if (!value) {
    return '';
  }
  if (measureTextWidth(value, font) <= maxWidth) {
    return value;
  }
  let prefix = '';
  for (const char of value) {
    const next = `${prefix}${char}`;
    if (prefix && measureTextWidth(next, font) > maxWidth) {
      break;
    }
    prefix = next;
  }
  return prefix || value.slice(0, 1);
};

const computeObstacleInsets = (obstacles, lineCenterY, left, width) => {
  let leftInset = 0;
  let rightInset = 0;
  const rectRight = left + width;
  const rectMid = left + width / 2;

  obstacles.forEach((obstacle) => {
    const radius = (obstacle.radius || 0) + 16;
    const weight = clamp(obstacle.weight || 0, 0, 1);
    if (radius <= 0 || weight <= 0.03) {
      return;
    }

    const dy = Math.abs(lineCenterY - obstacle.y);
    if (dy >= radius) {
      return;
    }

    const reach = Math.sqrt(Math.max(0, radius * radius - dy * dy));
    const occupiedLeft = obstacle.x - reach;
    const occupiedRight = obstacle.x + reach;
    if (occupiedRight <= left || occupiedLeft >= rectRight) {
      return;
    }

    const padding = 12 + weight * 8;
    if (obstacle.x >= rectMid) {
      rightInset = Math.max(rightInset, rectRight - occupiedLeft + padding);
      return;
    }
    leftInset = Math.max(leftInset, occupiedRight - left + padding);
  });

  return {
    leftInset: clamp(Math.round(leftInset), 0, Math.round(width * 0.45)),
    rightInset: clamp(Math.round(rightInset), 0, Math.round(width * 0.58)),
  };
};

const layoutTextAroundObstacles = (text, options) => {
  const content = String(text || '');
  const font = options?.font || TEXT_FONT;
  const lineHeight = options?.lineHeight || 22;
  const left = options?.left || 0;
  const top = options?.top || 0;
  const maxWidth = Math.max(140, options?.maxWidth || 240);
  const obstacles = Array.isArray(options?.obstacles) ? options.obstacles : [];
  const paragraphs = content.split(/\r?\n/);
  const lines = [];
  let cursorY = 0;
  let maxLeftInset = 0;
  let maxRightInset = 0;
  let widestContent = 0;

  const pushLine = (textValue) => {
    const centerY = top + cursorY + lineHeight / 2;
    const insets = computeObstacleInsets(obstacles, centerY, left, maxWidth);
    const available = Math.max(118, maxWidth - insets.leftInset - insets.rightInset);
    const cleaned = String(textValue || '').trimEnd();
    widestContent = Math.max(widestContent, measureTextWidth(cleaned || ' ', font) + insets.leftInset + insets.rightInset);
    maxLeftInset = Math.max(maxLeftInset, insets.leftInset);
    maxRightInset = Math.max(maxRightInset, insets.rightInset);
    lines.push({
      text: cleaned,
      y: cursorY,
      xOffset: insets.leftInset,
      availableWidth: available,
      leftInset: insets.leftInset,
      rightInset: insets.rightInset,
    });
    cursorY += lineHeight;
  };

  paragraphs.forEach((paragraph, index) => {
    const source = paragraph || '';
    if (!source.trim()) {
      pushLine('');
      cursorY -= lineHeight * 0.32;
      return;
    }

    let tokens = source.match(/\S+\s*/g) || [source];
    while (tokens.length) {
      const centerY = top + cursorY + lineHeight / 2;
      const insets = computeObstacleInsets(obstacles, centerY, left, maxWidth);
      const available = Math.max(118, maxWidth - insets.leftInset - insets.rightInset);
      let current = '';

      while (tokens.length) {
        const token = tokens[0] || '';
        const candidate = `${current}${token}`;
        if (!current && measureTextWidth(token, font) > available) {
          const prefix = takeFittingPrefix(token, available, font);
          current = prefix;
          const remainder = token.slice(prefix.length);
          if (remainder) {
            tokens[0] = remainder;
          } else {
            tokens.shift();
          }
          break;
        }
        if (!current || measureTextWidth(candidate, font) <= available) {
          current = candidate;
          tokens.shift();
          continue;
        }
        break;
      }

      pushLine(current);
    }

    if (index < paragraphs.length - 1) {
      cursorY += lineHeight * 0.18;
    }
  });

  return {
    lines,
    lineHeight,
    width: Math.min(maxWidth, widestContent),
    height: Math.max(lineHeight, cursorY),
    maxLeftInset,
    maxRightInset,
  };
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
      transition: { name: 'boot', startedAt: sceneClock() },
      statusChangedAt: sceneClock(),
    };
    this.destroyed = false;
    this.dpr = window.devicePixelRatio || 1;
    this.ctx = this.canvas?.getContext?.('2d');
    this.resizeObserver = null;
    this.resizeHandler = () => this.scheduleRender();
    this.source = null;
    this.frameHandle = 0;
    this.lastFrameAt = 0;
    this.lastMeasure = { width: 0, height: 420 };

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
      window.addEventListener('resize', this.resizeHandler, { passive: true });
      return;
    }

    this.resizeObserver = new ResizeObserver(() => this.scheduleRender());
    this.resizeObserver.observe(this.viewport);
  }

  requestFrame() {
    if (this.destroyed || this.frameHandle || !this.ctx) {
      return;
    }
    this.frameHandle = window.requestAnimationFrame((now) => this.tick(now));
  }

  scheduleRender() {
    this.updateCounters();
    this.requestFrame();
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

    if (snapshot.waiting_reason) {
      this.updateStatus('waiting');
      this.triggerTransition('waiting');
      if (!Object.keys(approvals).length) {
        this.pushNotice('Run waiting', snapshot.waiting_reason, Date.now());
      }
    }

    this.setLastEventMeta(seq, 'snapshot');
  }

  applyAGUIEvent(event, seq) {
    const timestamp = event.timestamp || Date.now();
    switch (event.type) {
      case 'RUN_STARTED':
        this.runId = event.runId || this.runId;
        this.updateStatus('running');
        this.triggerTransition('resumed');
        this.pushNotice('Run started', this.runId || 'live session', timestamp);
        break;
      case 'RUN_FINISHED':
        this.updateStatus('completed');
        this.triggerTransition('finished');
        this.pushNotice('Run finished', this.runId || 'completed', timestamp);
        break;
      case 'RUN_ERROR':
        this.updateStatus('failed');
        this.triggerTransition('error');
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
        this.triggerTransition('waiting');
        this.pushNotice('Run waiting', payload.reason || 'paused', timestamp);
        break;
      case 'gollem.run.resumed':
        this.updateStatus('running');
        this.triggerTransition('resumed');
        this.pushNotice('Run resumed', payload.runId || this.runId || 'stream resumed', timestamp);
        break;
      case 'gollem.approval.requested': {
        const tool = this.ensureToolBody(payload.toolCallId || `approval_${Date.now()}`, payload.toolName || 'approval', timestamp);
        if (tool) {
          tool.status = 'approval';
          tool.args = compactWhitespace(payload.argsJson || tool.args || '');
        }
        this.updateStatus('waiting');
        this.triggerTransition('waiting');
        break;
      }
      case 'gollem.approval.resolved': {
        const tool = this.scene.toolBodies.get(payload.toolCallId || '');
        if (tool) {
          tool.status = payload.approved ? 'approved' : 'denied';
          tool.updatedAt = timestamp;
          tool.resolvedAt = sceneClock();
        }
        this.updateStatus('running');
        this.triggerTransition('resumed');
        break;
      }
      case 'gollem.deferred.requested':
        this.updateStatus('waiting');
        this.triggerTransition('waiting');
        this.pushNotice('Deferred input', payload.toolName || payload.toolCallId || 'awaiting input', timestamp);
        break;
      case 'gollem.deferred.resolved':
        this.updateStatus('running');
        this.triggerTransition('resumed');
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

  triggerTransition(name) {
    this.scene.transition = { name, startedAt: sceneClock() };
    const kick = name === 'waiting'
      ? { x: -10, y: -22 }
      : name === 'resumed'
        ? { x: 18, y: -36 }
        : name === 'finished'
          ? { x: 0, y: 20 }
          : name === 'error'
            ? { x: 28, y: -8 }
            : { x: 0, y: 0 };

    this.scene.flow.forEach((item) => {
      item.vx = (item.vx || 0) + (item.kind === 'tool' ? kick.x : kick.x * 0.35) * (item.order % 2 === 0 ? -1 : 1);
      item.vy = (item.vy || 0) + kick.y * (item.kind === 'tool' ? 1 : 0.45);
      if (item.kind === 'tool' && name === 'waiting') {
        item.pulseBoost = 1.2;
      }
    });
  }

  updateStatus(status) {
    const next = status || this.scene.runStatus || 'running';
    const changed = next !== this.scene.runStatus;
    this.scene.runStatus = next;
    if (changed) {
      this.scene.statusChangedAt = sceneClock();
      this.root.dataset.sceneStatus = next;
    }
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
    this.root.dataset.sceneConnection = connection;
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
    const now = sceneClock();
    const record = {
      ...item,
      order: this.scene.flow.length + 1,
      alpha: typeof item.alpha === 'number' ? item.alpha : 1,
      targetAlpha: typeof item.targetAlpha === 'number' ? item.targetAlpha : 1,
      appearedAt: typeof item.appearedAt === 'number' ? item.appearedAt : now,
      pulseBoost: typeof item.pulseBoost === 'number' ? item.pulseBoost : 0,
      vx: 0,
      vy: 0,
    };
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
      appearedAt: sceneClock(),
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
        appearedAt: sceneClock(),
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
        appearedAt: sceneClock(),
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
        appearedAt: sceneClock(),
        radius: 0,
        targetRadius: 0,
      });
      this.scene.toolBodies.set(toolCallId, tool);
      tool.vx = (tool.order % 2 === 0 ? -1 : 1) * 18;
      tool.vy = -22;
    }
    if (toolCallName) {
      tool.title = toolCallName;
    }
    tool.updatedAt = timestamp;
    if (!tool.stepName && this.scene.activeStepName) {
      tool.stepName = this.scene.activeStepName;
    }
    tool.resolvedAt = tool.status === 'returned' || tool.status === 'failed' ? tool.resolvedAt : 0;
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
    tool.resolvedAt = sceneClock();
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

  measureViewport() {
    const bounds = this.viewport.getBoundingClientRect();
    const width = Math.max(320, Math.round(bounds.width || this.canvas.clientWidth || 320));
    this.lastMeasure.width = width;
    return width;
  }

  toolResolveProgress(item, now) {
    if (!item?.resolvedAt) {
      return 0;
    }
    const duration = item.status === 'failed' ? 1500 : 900;
    return clamp((now - item.resolvedAt) / duration, 0, 1);
  }

  toolLayoutWeight(item, now) {
    if (!item) {
      return 0;
    }
    if (item.status === 'failed') {
      return 1 - this.toolResolveProgress(item, now) * 0.72;
    }
    if (item.status === 'returned' || item.status === 'approved' || item.status === 'denied') {
      return 1 - this.toolResolveProgress(item, now);
    }
    return 1;
  }

  toolRadiusTarget(item, now) {
    const previewSize = Math.max(item.args?.length || 0, item.result?.length || 0);
    const base = clamp(44 + Math.round(previewSize / 24), 42, 84);
    const statusBoost = item.status === 'approval' ? 8 : item.status === 'failed' ? 10 : item.status === 'returned' ? -6 : 0;
    const resolvedShrink = (item.status === 'returned' || item.status === 'approved' || item.status === 'denied' || item.status === 'failed')
      ? lerp(1, item.status === 'failed' ? 0.48 : 0.12, this.toolResolveProgress(item, now))
      : 1;
    return Math.max(12, (base + statusBoost) * resolvedShrink);
  }

  toolAlphaTarget(item, now) {
    const base = item.status === 'failed' ? 0.9 : 1;
    if (item.status === 'returned' || item.status === 'approved' || item.status === 'denied' || item.status === 'failed') {
      return lerp(base, 0.04, this.toolResolveProgress(item, now));
    }
    return base;
  }

  buildToolPreview(item) {
    const parts = [];
    if (item.args) {
      parts.push(`args ${summarize(item.args, 84)}`);
    }
    if (item.result) {
      parts.push(`result ${summarize(item.result, 84)}`);
    }
    if (!parts.length) {
      parts.push(item.status || 'pending');
    }
    return parts.join('\n');
  }

  buildTextLayout(item, left, top, width, obstacles) {
    const font = item.kind === 'reasoning' ? MONO_FONT : TEXT_FONT;
    const lineHeight = item.kind === 'reasoning' ? 20 : 22;
    const content = item.kind === 'notice' ? (item.detail || item.content || item.title) : (item.content || '…');
    const textLeft = left + 28;
    const textTop = top + 48;
    const layout = layoutTextAroundObstacles(content, {
      left: textLeft,
      top: textTop,
      maxWidth: Math.max(168, width - 56),
      font,
      lineHeight,
      obstacles,
    });
    const pressure = Math.max(layout.maxRightInset, Math.round(layout.maxLeftInset * 0.55));
    const widthReduction = clamp(Math.round(pressure * 0.56), 0, Math.round(width * 0.26));
    return {
      ...layout,
      targetWidth: Math.max(236, width - widthReduction),
      targetHeight: 70 + layout.height,
    };
  }

  computeLayout(width, now) {
    const compactHeader = width < 680;
    const headerHeight = compactHeader ? 96 : 64;
    const pad = clamp(Math.round(width * 0.038), 18, 34);
    const contentLeft = pad + 18;
    const contentWidth = Math.max(244, width - contentLeft - pad - 8);
    const narrativeWidth = Math.max(244, contentWidth - (compactHeader ? 8 : 18));
    const gap = width < 760 ? 16 : 20;
    let cursorY = 18 + headerHeight + 22;
    let lastNarrativeY = cursorY;
    let maxBottom = cursorY;
    const items = [];
    const activeTools = [];

    this.scene.flow.forEach((item) => {
      if (item.kind === 'tool') {
        const toolIndex = activeTools.length;
        const radius = this.toolRadiusTarget(item, now);
        const wobble = toolIndex % 2 === 0 ? 0 : 10;
        const rise = Math.min(54, toolIndex * 18);
        item.targetRadius = radius;
        item.targetAlpha = this.toolAlphaTarget(item, now);
        item.layoutWeight = this.toolLayoutWeight(item, now);
        item.linkY = lastNarrativeY;
        item.linkX = contentLeft + Math.max(160, narrativeWidth - 140);
        item.targetX = clamp(width - pad - radius - 18 - wobble, contentLeft + radius + 120, width - pad - radius - 10);
        item.targetY = Math.max(36 + headerHeight, lastNarrativeY + 12 + rise);
        items.push(item);
        activeTools.push(item);
        maxBottom = Math.max(maxBottom, item.targetY + radius + 18);
        cursorY = Math.max(cursorY, item.targetY - radius * 0.24);
        return;
      }

      if (item.kind === 'step') {
        item.targetX = contentLeft;
        item.targetY = cursorY;
        item.targetWidth = contentWidth;
        item.targetHeight = 40;
        cursorY += item.targetHeight + gap;
        lastNarrativeY = item.targetY + item.targetHeight / 2;
        maxBottom = Math.max(maxBottom, item.targetY + item.targetHeight);
        items.push(item);
        return;
      }

      const inset = item.kind === 'reasoning' ? 26 : item.kind === 'notice' ? 10 : 0;
      const obstacles = activeTools
        .map((tool) => ({
          x: Number.isFinite(tool.x) ? tool.x : tool.targetX,
          y: Number.isFinite(tool.y) ? tool.y : tool.targetY,
          radius: Number.isFinite(tool.radius) ? tool.radius : tool.targetRadius,
          weight: this.toolLayoutWeight(tool, now),
        }))
        .filter((tool) => tool.weight > 0.06 && tool.radius > 8);

      const baseLeft = contentLeft + inset;
      const baseWidth = Math.max(240, narrativeWidth - inset);
      const layout = this.buildTextLayout(item, baseLeft, cursorY, baseWidth, obstacles);
      item.textLayout = layout;
      item.targetX = baseLeft;
      item.targetY = cursorY;
      item.targetWidth = layout.targetWidth;
      item.targetHeight = layout.targetHeight;
      cursorY += item.targetHeight + gap;
      lastNarrativeY = item.targetY + item.targetHeight / 2;
      maxBottom = Math.max(maxBottom, item.targetY + item.targetHeight);
      items.push(item);
    });

    return {
      width,
      pad,
      headerHeight,
      compactHeader,
      items,
      height: Math.max(440, Math.round(Math.max(cursorY + 34, maxBottom + 24))),
    };
  }

  advanceCard(item, dt, width, height) {
    if (!Number.isFinite(item.x)) {
      item.x = item.targetX;
    }
    if (!Number.isFinite(item.y)) {
      item.y = item.targetY;
    }
    if (!Number.isFinite(item.displayWidth)) {
      item.displayWidth = item.targetWidth;
    }
    if (!Number.isFinite(item.displayHeight)) {
      item.displayHeight = item.targetHeight;
    }
    item.alpha = typeof item.alpha === 'number' ? item.alpha : 1;
    item.targetAlpha = typeof item.targetAlpha === 'number' ? item.targetAlpha : 1;

    const stiffness = item.kind === 'step' ? 18 : 14;
    const damping = item.kind === 'step' ? 0.78 : 0.8;
    const gravity = item.kind === 'step' ? 26 : 44;
    item.vx = (item.vx || 0) + (item.targetX - item.x) * stiffness * dt;
    item.vy = (item.vy || 0) + (item.targetY - item.y) * stiffness * dt + gravity * dt;
    item.vx *= Math.pow(damping, dt * 60);
    item.vy *= Math.pow(damping, dt * 60);
    item.x += item.vx * dt * 60;
    item.y += item.vy * dt * 60;
    item.displayWidth = lerp(item.displayWidth, item.targetWidth, smoothAmount(14, dt));
    item.displayHeight = lerp(item.displayHeight, item.targetHeight, smoothAmount(14, dt));
    item.alpha = lerp(item.alpha, item.targetAlpha, smoothAmount(10, dt));

    const maxX = width - this.lastMeasure.width * 0 + width - width;
    void maxX;
    const minX = 12;
    const boundRight = width - item.displayWidth - 12;
    const boundBottom = height - item.displayHeight - 10;
    if (item.x < minX) {
      item.x = minX;
      item.vx *= -0.32;
    }
    if (item.x > boundRight) {
      item.x = boundRight;
      item.vx *= -0.32;
    }
    if (item.y < 14) {
      item.y = 14;
      item.vy *= -0.28;
    }
    if (item.y > boundBottom) {
      item.y = boundBottom;
      item.vy *= -0.26;
    }

    return Math.abs(item.vx) > 0.08 || Math.abs(item.vy) > 0.08 || !motionSettled(item.x, item.targetX) || !motionSettled(item.y, item.targetY) || !motionSettled(item.displayWidth, item.targetWidth);
  }

  advanceTool(item, dt, width, height, now) {
    if (!Number.isFinite(item.x)) {
      item.x = item.targetX;
    }
    if (!Number.isFinite(item.y)) {
      item.y = item.targetY;
    }
    if (!Number.isFinite(item.radius)) {
      item.radius = item.targetRadius;
    }
    item.alpha = typeof item.alpha === 'number' ? item.alpha : 1;

    const status = this.scene.runStatus;
    const gravity = status === 'waiting' ? 18 : status === 'failed' ? 72 : status === 'completed' ? 12 : 32;
    const stiffness = status === 'waiting' ? 8 : 10;
    const damping = status === 'failed' ? 0.74 : 0.8;
    const pulse = 0.4 + Math.sin(now / 280 + item.order * 0.9) * 0.5;
    item.pulseBoost = lerp(item.pulseBoost || 0, 0, smoothAmount(8, dt));

    item.vx = (item.vx || 0) + (item.targetX - item.x) * stiffness * dt;
    item.vy = (item.vy || 0) + (item.targetY - item.y) * stiffness * dt + gravity * dt - pulse * 0.45;
    item.vx *= Math.pow(damping, dt * 60);
    item.vy *= Math.pow(damping, dt * 60);
    item.x += item.vx * dt * 60;
    item.y += item.vy * dt * 60;
    item.radius = lerp(item.radius, item.targetRadius, smoothAmount(12, dt));
    item.alpha = lerp(item.alpha, item.targetAlpha, smoothAmount(10, dt));

    const minX = 24 + item.radius;
    const maxX = width - 20 - item.radius;
    const minY = 18 + this.lastLayout.headerHeight + item.radius * 0.42;
    const maxY = height - 16 - item.radius;
    if (item.x < minX) {
      item.x = minX;
      item.vx = Math.abs(item.vx) * 0.66;
    }
    if (item.x > maxX) {
      item.x = maxX;
      item.vx = -Math.abs(item.vx) * 0.66;
    }
    if (item.y < minY) {
      item.y = minY;
      item.vy = Math.abs(item.vy) * 0.58;
    }
    if (item.y > maxY) {
      item.y = maxY;
      item.vy = -Math.abs(item.vy) * 0.72;
    }

    return item.layoutWeight > 0.04 || item.alpha > 0.05 || Math.abs(item.vx) > 0.08 || Math.abs(item.vy) > 0.08 || !motionSettled(item.x, item.targetX, 1.2) || !motionSettled(item.y, item.targetY, 1.2);
  }

  advance(layout, now, dt) {
    let moving = false;
    this.lastLayout = layout;
    layout.items.forEach((item) => {
      item.targetAlpha = typeof item.targetAlpha === 'number' ? item.targetAlpha : 1;
      if (item.kind === 'tool') {
        moving = this.advanceTool(item, dt, layout.width, layout.height, now) || moving;
        return;
      }
      moving = this.advanceCard(item, dt, layout.width, layout.height) || moving;
    });

    const phaseAge = now - (this.scene.transition?.startedAt || now);
    const transitionAlive = phaseAge < 2200;
    const streamingAlive = this.scene.runStatus === 'running' || this.scene.runStatus === 'waiting' || this.scene.connection === 'reconnecting';
    return moving || transitionAlive || streamingAlive;
  }

  tick(now) {
    this.frameHandle = 0;
    if (this.destroyed || !this.ctx || !this.viewport) {
      return;
    }

    const width = this.measureViewport();
    const layout = this.computeLayout(width, now);
    this.resizeCanvas(width, layout.height);
    const dt = clamp(((now - this.lastFrameAt) || 16.6667) / 1000, 1 / 120, 0.05);
    this.lastFrameAt = now;
    const active = this.advance(layout, now, dt);
    this.render(layout, now);

    if (active) {
      this.requestFrame();
    }
  }

  renderBackground(width, height, now) {
    const transitionName = this.scene.transition?.name || this.scene.runStatus;
    const accent = transitionName === 'waiting'
      ? 'rgba(251, 191, 36, 0.12)'
      : transitionName === 'finished'
        ? 'rgba(52, 211, 153, 0.12)'
        : transitionName === 'error'
          ? 'rgba(248, 113, 113, 0.12)'
          : 'rgba(125, 211, 252, 0.12)';

    const gradient = this.ctx.createLinearGradient(0, 0, width, height);
    gradient.addColorStop(0, '#040816');
    gradient.addColorStop(0.52, '#091120');
    gradient.addColorStop(1, '#040816');
    this.ctx.fillStyle = gradient;
    this.ctx.fillRect(0, 0, width, height);

    const glow = this.ctx.createRadialGradient(width * 0.24, 24, 12, width * 0.24, 24, width * 0.68);
    glow.addColorStop(0, accent);
    glow.addColorStop(0.7, 'rgba(56, 189, 248, 0.03)');
    glow.addColorStop(1, 'rgba(56, 189, 248, 0)');
    this.ctx.fillStyle = glow;
    this.ctx.fillRect(0, 0, width, height);

    this.ctx.fillStyle = 'rgba(148, 163, 184, 0.18)';
    const dotSpacing = 24;
    for (let x = 20; x < width; x += dotSpacing) {
      for (let y = 20; y < height; y += dotSpacing) {
        const phase = Math.sin((now / 900) + x * 0.02 + y * 0.015) * 0.06;
        this.ctx.globalAlpha = 0.35 + phase;
        this.ctx.fillRect(x, y, 1.4, 1.4);
      }
    }
    this.ctx.globalAlpha = 1;

    this.ctx.fillStyle = 'rgba(8, 15, 30, 0.92)';
    this.ctx.fillRect(0, 0, width, 18);
    this.ctx.fillRect(0, 0, 18, height);
    this.ctx.strokeStyle = 'rgba(125, 211, 252, 0.12)';
    this.ctx.lineWidth = 1;
    this.ctx.beginPath();
    this.ctx.moveTo(18.5, 0);
    this.ctx.lineTo(18.5, height);
    this.ctx.moveTo(0, 18.5);
    this.ctx.lineTo(width, 18.5);
    this.ctx.stroke();

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = 'rgba(148, 163, 184, 0.72)';
    this.ctx.textBaseline = 'middle';
    for (let x = 48; x < width; x += 96) {
      this.ctx.beginPath();
      this.ctx.moveTo(x + 0.5, 0);
      this.ctx.lineTo(x + 0.5, 12);
      this.ctx.stroke();
      this.ctx.fillText(String(x).padStart(3, '0'), x - 10, 10);
    }
    for (let y = 56; y < height; y += 96) {
      this.ctx.beginPath();
      this.ctx.moveTo(0, y + 0.5);
      this.ctx.lineTo(12, y + 0.5);
      this.ctx.stroke();
      this.ctx.save();
      this.ctx.translate(10, y + 10);
      this.ctx.rotate(-Math.PI / 2);
      this.ctx.fillText(String(y).padStart(3, '0'), 0, 0);
      this.ctx.restore();
    }
  }

  renderSceneFx(width, height, now) {
    const age = now - (this.scene.transition?.startedAt || now);
    const progress = clamp(age / 1400, 0, 1);
    const pulse = 0.5 + Math.sin(now / 180) * 0.5;
    const name = this.scene.transition?.name || this.scene.runStatus;

    if (name === 'waiting') {
      this.ctx.fillStyle = `rgba(251, 191, 36, ${0.05 + pulse * 0.03})`;
      this.ctx.fillRect(18, 18, width - 18, height - 18);
      return;
    }
    if (name === 'resumed') {
      this.ctx.strokeStyle = `rgba(125, 211, 252, ${0.26 * (1 - progress)})`;
      this.ctx.lineWidth = 2;
      const sweepX = lerp(24, width - 24, progress);
      this.ctx.beginPath();
      this.ctx.moveTo(sweepX, 20);
      this.ctx.lineTo(sweepX - 40, height - 18);
      this.ctx.stroke();
      return;
    }
    if (name === 'finished') {
      this.ctx.fillStyle = `rgba(52, 211, 153, ${0.14 * (1 - progress)})`;
      this.ctx.fillRect(18, 18, width - 18, height - 18);
      return;
    }
    if (name === 'error') {
      this.ctx.fillStyle = `rgba(248, 113, 113, ${0.12 * (1 - progress)})`;
      this.ctx.fillRect(18, 18, width - 18, height - 18);
    }
  }

  renderHeader(width) {
    const compactHeader = width < 680;
    const headerHeight = compactHeader ? 96 : 64;
    this.ctx.fillStyle = 'rgba(3, 7, 18, 0.72)';
    drawRoundedRect(this.ctx, 26, 24, width - 52, headerHeight, 18);
    this.ctx.fill();
    this.ctx.strokeStyle = 'rgba(125, 211, 252, 0.16)';
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    this.ctx.font = TITLE_FONT;
    this.ctx.fillStyle = '#f8fafc';
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(this.title, 42, 44);

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = '#94a3b8';
    this.ctx.fillText(summarize(this.summary || this.runId || 'Live AG-UI stream', compactHeader ? 72 : 74), 42, 62);
    this.ctx.fillText('PRETEXT FIELD · AG-UI STREAM', 42, compactHeader ? 84 : 82);

    const statusColor = this.scene.runStatus === 'completed'
      ? '#34d399'
      : this.scene.runStatus === 'failed'
        ? '#fda4af'
        : this.scene.runStatus === 'waiting'
          ? '#fbbf24'
          : '#7dd3fc';

    if (compactHeader) {
      const statusWidth = drawPill(this.ctx, 42, 84, this.scene.runStatus, 'rgba(15, 23, 42, 0.96)', 'rgba(125, 211, 252, 0.2)', statusColor);
      drawPill(this.ctx, 42 + statusWidth + 10, 84, this.scene.connection, 'rgba(15, 23, 42, 0.96)', 'rgba(148, 163, 184, 0.22)', '#cbd5e1');
      return;
    }

    const statusWidth = measurePillWidth(this.scene.runStatus);
    const connectionWidth = measurePillWidth(this.scene.connection);
    const totalWidth = statusWidth + connectionWidth + 10;
    const statusX = Math.max(42, width - 42 - totalWidth);
    const drawnStatusWidth = drawPill(this.ctx, statusX, 34, this.scene.runStatus, 'rgba(15, 23, 42, 0.96)', 'rgba(125, 211, 252, 0.2)', statusColor);
    drawPill(this.ctx, statusX + drawnStatusWidth + 10, 34, this.scene.connection, 'rgba(15, 23, 42, 0.96)', 'rgba(148, 163, 184, 0.22)', '#cbd5e1');
  }

  renderStep(item) {
    const x = item.x;
    const y = item.y;
    const width = item.displayWidth;
    const height = item.displayHeight;
    drawRoundedRect(this.ctx, x, y, width, height, 16);
    this.ctx.fillStyle = 'rgba(12, 20, 36, 0.84)';
    this.ctx.fill();
    this.ctx.strokeStyle = item.status === 'completed' ? 'rgba(52, 211, 153, 0.34)' : 'rgba(125, 211, 252, 0.24)';
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    this.ctx.fillStyle = item.status === 'completed' ? '#86efac' : '#7dd3fc';
    this.ctx.font = META_FONT;
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(`STEP ${item.index}`, x + 16, y + height / 2 + 0.5);

    this.ctx.font = TITLE_FONT;
    this.ctx.fillStyle = '#e2e8f0';
    this.ctx.fillText(item.title, x + 82, y + height / 2 + 0.5);

    this.ctx.fillStyle = '#64748b';
    this.ctx.fillText(formatClock(item.updatedAt), x + width - 78, y + height / 2 + 0.5);
  }

  renderTextCard(item, now) {
    const x = item.x;
    const y = item.y;
    const width = item.displayWidth;
    const height = item.displayHeight;
    const accent = item.kind === 'reasoning' ? '#c084fc' : item.kind === 'notice' ? '#fbbf24' : '#7dd3fc';
    const border = item.kind === 'reasoning' ? 'rgba(192, 132, 252, 0.3)' : item.kind === 'notice' ? 'rgba(251, 191, 36, 0.28)' : 'rgba(125, 211, 252, 0.24)';
    const fill = item.kind === 'reasoning' ? 'rgba(36, 18, 59, 0.58)' : item.kind === 'notice' ? 'rgba(56, 44, 18, 0.52)' : 'rgba(12, 20, 36, 0.88)';
    const pulse = item.status === 'streaming' ? 0.06 + (Math.sin(now / 160) * 0.5 + 0.5) * 0.08 : 0;

    drawRoundedRect(this.ctx, x, y, width, height, 20);
    this.ctx.fillStyle = fill;
    this.ctx.fill();
    this.ctx.strokeStyle = border;
    this.ctx.lineWidth = 1;
    this.ctx.stroke();

    if (pulse > 0) {
      this.ctx.fillStyle = `rgba(125, 211, 252, ${pulse})`;
      this.ctx.fill();
    }

    this.ctx.fillStyle = accent;
    this.ctx.fillRect(x + 16, y + 16, 4, height - 32);
    drawPill(this.ctx, x + 30, y + 14, roleLabel(item.role, item.kind), 'rgba(2, 6, 23, 0.52)', border, accent);

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = '#94a3b8';
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(formatClock(item.updatedAt), x + width - 74, y + 25.5);

    this.ctx.font = item.kind === 'reasoning' ? MONO_FONT : TEXT_FONT;
    this.ctx.fillStyle = item.kind === 'notice' ? '#fde68a' : '#e2e8f0';
    this.ctx.textBaseline = 'top';
    const startX = x + 28;
    const startY = y + 48;
    const layout = item.textLayout || { lines: [], lineHeight: item.kind === 'reasoning' ? 20 : 22 };
    layout.lines.forEach((line) => {
      this.ctx.fillText(line.text || ' ', startX + (line.xOffset || 0), startY + line.y);
    });
  }

  renderTool(item, now) {
    if ((item.alpha || 0) <= 0.03 || (item.radius || 0) <= 4) {
      return;
    }

    const resolveProgress = this.toolResolveProgress(item, now);
    const stroke = item.status === 'failed' ? 'rgba(248, 113, 113, 0.4)' : item.status === 'returned' ? 'rgba(52, 211, 153, 0.36)' : item.status === 'approval' ? 'rgba(251, 191, 36, 0.36)' : 'rgba(125, 211, 252, 0.3)';
    const fill = item.status === 'failed' ? 'rgba(69, 10, 10, 0.68)' : 'rgba(11, 17, 32, 0.94)';
    const markerColor = item.status === 'failed' ? '#f87171' : item.status === 'returned' ? '#34d399' : item.status === 'approval' ? '#fbbf24' : '#7dd3fc';
    const pulse = 0.5 + Math.sin(now / 220 + item.order * 0.7) * 0.5;
    const outerRadius = item.radius + 8 + pulse * (item.status === 'approval' || this.scene.runStatus === 'waiting' ? 7 : 3);

    this.ctx.save();
    this.ctx.globalAlpha = clamp(item.alpha, 0, 1);

    this.ctx.strokeStyle = `rgba(148, 163, 184, ${0.22 * Math.max(0.2, item.alpha)})`;
    this.ctx.lineWidth = 1.4;
    this.ctx.beginPath();
    this.ctx.moveTo(item.linkX || item.x - item.radius - 20, item.linkY || item.y);
    this.ctx.lineTo(item.x - item.radius * 0.2, item.y);
    this.ctx.stroke();

    this.ctx.beginPath();
    this.ctx.arc(item.x, item.y, outerRadius, 0, Math.PI * 2);
    this.ctx.strokeStyle = markerColor;
    this.ctx.globalAlpha = clamp(item.alpha * (0.16 + pulse * 0.16 + (item.pulseBoost || 0) * 0.08), 0, 1);
    this.ctx.lineWidth = 1.2;
    this.ctx.stroke();
    this.ctx.globalAlpha = clamp(item.alpha, 0, 1);

    const radial = this.ctx.createRadialGradient(item.x - item.radius * 0.24, item.y - item.radius * 0.32, item.radius * 0.2, item.x, item.y, item.radius * 1.2);
    radial.addColorStop(0, item.status === 'failed' ? 'rgba(248, 113, 113, 0.26)' : 'rgba(125, 211, 252, 0.2)');
    radial.addColorStop(1, fill);
    this.ctx.beginPath();
    this.ctx.arc(item.x, item.y, item.radius, 0, Math.PI * 2);
    this.ctx.fillStyle = radial;
    this.ctx.fill();
    this.ctx.strokeStyle = stroke;
    this.ctx.lineWidth = 1.2;
    this.ctx.stroke();

    if (item.status === 'returned' || item.status === 'failed') {
      this.ctx.beginPath();
      this.ctx.arc(item.x, item.y, item.radius + 18 + resolveProgress * 24, 0, Math.PI * 2);
      this.ctx.strokeStyle = item.status === 'failed' ? `rgba(248, 113, 113, ${0.24 * (1 - resolveProgress)})` : `rgba(52, 211, 153, ${0.24 * (1 - resolveProgress)})`;
      this.ctx.lineWidth = 1.4;
      this.ctx.stroke();
    }

    this.ctx.font = TOOL_TITLE_FONT;
    this.ctx.fillStyle = '#f8fafc';
    this.ctx.textBaseline = 'middle';
    this.ctx.textAlign = 'center';
    this.ctx.fillText(summarize(item.title || 'tool', 18), item.x, item.y - item.radius * 0.26);

    this.ctx.font = META_FONT;
    this.ctx.fillStyle = markerColor;
    this.ctx.fillText((item.status || 'pending').toUpperCase(), item.x, item.y + item.radius * 0.02);

    const preview = ensurePretextLayout(this.buildToolPreview(item), {
      maxWidth: Math.max(70, item.radius * 1.5),
      font: MONO_FONT,
      lineHeight: 14,
    });
    this.ctx.font = MONO_FONT;
    this.ctx.fillStyle = '#cbd5e1';
    preview.lines.slice(0, 3).forEach((line, index) => {
      this.ctx.fillText(summarize(line || ' ', 18), item.x, item.y + item.radius * 0.24 + index * 14);
    });

    this.ctx.textAlign = 'start';
    this.ctx.restore();
  }

  render(layout, now) {
    if (!this.ctx || !this.viewport) {
      return;
    }

    this.lastLayout = layout;
    const width = layout.width;
    const height = layout.height;
    const errorShake = this.scene.transition?.name === 'error' ? Math.sin(now / 28) * (1 - clamp((now - this.scene.transition.startedAt) / 1200, 0, 1)) * 3.2 : 0;

    this.ctx.save();
    this.ctx.translate(errorShake, 0);
    this.renderBackground(width, height, now);
    this.renderSceneFx(width, height, now);
    this.renderHeader(width);

    layout.items.forEach((item) => {
      if (item.kind === 'step') {
        this.renderStep(item);
      }
    });
    layout.items.forEach((item) => {
      if (item.kind !== 'step' && item.kind !== 'tool') {
        this.renderTextCard(item, now);
      }
    });
    layout.items.forEach((item) => {
      if (item.kind === 'tool') {
        this.renderTool(item, now);
      }
    });
    this.ctx.restore();
  }

  destroy() {
    this.destroyed = true;
    if (this.frameHandle) {
      window.cancelAnimationFrame(this.frameHandle);
      this.frameHandle = 0;
    }
    if (this.source) {
      this.source.close();
      this.source = null;
    }
    if (this.resizeObserver) {
      this.resizeObserver.disconnect();
      this.resizeObserver = null;
    } else {
      window.removeEventListener('resize', this.resizeHandler);
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
