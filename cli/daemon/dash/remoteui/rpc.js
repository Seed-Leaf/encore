// Dependency-free JSON-RPC transport for the remote read-only dashboard.
export class DashboardRPC {
  constructor(url, options = {}) {
    this.url = url;
    this.makeSocket = options.makeSocket || ((address) => new WebSocket(address));
    this.setTimer = options.setTimer || ((fn, delay) => setTimeout(fn, delay));
    this.clearTimer = options.clearTimer || ((timer) => clearTimeout(timer));
    this.onState = options.onState || (() => {});
    this.onNotification = options.onNotification || (() => {});
    this.requestTimeout = options.requestTimeout ?? 10000;
    this.connectTimeout = options.connectTimeout ?? 8000;
    this.maxRetries = options.maxRetries ?? 6;
    this.pending = new Map();
    this.nextID = 1;
    this.attempt = 0;
    this.socket = null;
    this.retryTimer = null;
    this.connectTimer = null;
    this.stopped = true;
  }

  get connected() { return this.socket?.readyState === 1; }

  start() {
    this.stop();
    this.stopped = false;
    this.attempt = 0;
    this.connect();
  }

  connect() {
    if (this.stopped) return;
    this.onState({ state: "connecting", attempt: this.attempt });
    let socket;
    try {
      socket = this.makeSocket(this.url);
    } catch {
      this.scheduleRetry();
      return;
    }
    this.socket = socket;
    this.connectTimer = this.setTimer(() => this.disconnect(socket, "Connection timed out."), this.connectTimeout);
    socket.addEventListener("open", () => {
      if (this.socket !== socket || this.stopped) return;
      this.clearTimer(this.connectTimer);
      this.connectTimer = null;
      this.attempt = 0;
      this.onState({ state: "connected" });
    });
    socket.addEventListener("message", (event) => {
      if (this.socket !== socket) return;
      let message;
      try { message = JSON.parse(event.data); } catch {
        this.disconnect(socket, "The server sent an invalid response.");
        return;
      }
      if (!message || typeof message !== "object" || message.jsonrpc !== "2.0") return;
      if (Object.hasOwn(message, "id")) {
        const request = this.pending.get(message.id);
        if (!request) return;
        this.pending.delete(message.id);
        this.clearTimer(request.timer);
        if (message.error) {
          request.reject(new Error(String(message.error.message || "The request was rejected.")));
        } else if (Object.hasOwn(message, "result")) {
          request.resolve(message.result);
        } else {
          request.reject(new Error("The server returned an incomplete response."));
        }
      } else if (typeof message.method === "string") {
        this.onNotification(message);
      }
    });
    socket.addEventListener("close", () => this.disconnect(socket, "Connection closed."));
    socket.addEventListener("error", () => this.disconnect(socket, "Connection unavailable."));
  }

  disconnect(socket, reason) {
    if (this.socket !== socket) return;
    this.socket = null;
    this.clearTimer(this.connectTimer);
    this.connectTimer = null;
    this.rejectPending(reason);
    try { socket.close(); } catch { /* Already closed. */ }
    this.scheduleRetry();
  }

  scheduleRetry() {
    if (this.stopped) return;
    if (this.attempt >= this.maxRetries) {
      this.onState({ state: "offline" });
      return;
    }
    const delay = Math.min(1000 * 2 ** this.attempt, 15000);
    this.attempt += 1;
    this.onState({ state: "reconnecting", attempt: this.attempt, delay });
    this.retryTimer = this.setTimer(() => {
      this.retryTimer = null;
      this.connect();
    }, delay);
  }

  request(method, params) {
    if (!this.connected) return Promise.reject(new Error("The dashboard is not connected. Try reconnecting."));
    if (this.pending.size >= 64) return Promise.reject(new Error("Too many pending requests. Try again shortly."));
    const id = this.nextID++;
    return new Promise((resolve, reject) => {
      const timer = this.setTimer(() => {
        this.pending.delete(id);
        reject(new Error("The request timed out. Refresh to try again."));
      }, this.requestTimeout);
      this.pending.set(id, { resolve, reject, timer });
      try {
        this.socket.send(JSON.stringify({ jsonrpc: "2.0", id, method, ...(params === undefined ? {} : { params }) }));
      } catch {
        this.pending.delete(id);
        this.clearTimer(timer);
        reject(new Error("The request could not be sent. Try reconnecting."));
      }
    });
  }

  rejectPending(reason) {
    for (const request of this.pending.values()) {
      this.clearTimer(request.timer);
      request.reject(new Error(reason));
    }
    this.pending.clear();
  }

  stop() {
    this.stopped = true;
    this.clearTimer(this.retryTimer);
    this.clearTimer(this.connectTimer);
    this.retryTimer = this.connectTimer = null;
    const socket = this.socket;
    this.socket = null;
    this.rejectPending("Connection closed.");
    if (socket) { try { socket.close(); } catch { /* Already closed. */ } }
  }
}

export function resultList(value, allowNull = false) {
  if (allowNull && value === null) return [];
  if (!Array.isArray(value)) throw new Error("The server returned an unexpected list response.");
  return value;
}
