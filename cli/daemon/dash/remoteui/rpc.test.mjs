import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// Import browser ESM without changing this repository's Node package settings.
const source = await readFile(new URL("./rpc.js", import.meta.url), "utf8");
const { DashboardRPC, resultList } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

class Clock {
  now = 0;
  nextID = 1;
  timers = new Map();
  set = (fn, delay) => {
    const id = this.nextID++;
    this.timers.set(id, { at: this.now + delay, fn });
    return id;
  };
  clear = (id) => this.timers.delete(id);
  advance(ms) {
    const end = this.now + ms;
    for (;;) {
      const next = [...this.timers].filter(([, timer]) => timer.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
      if (!next) break;
      this.now = next[1].at;
      this.timers.delete(next[0]);
      next[1].fn();
    }
    this.now = end;
  }
}

class Socket {
  readyState = 0;
  listeners = new Map();
  sent = [];
  addEventListener(name, fn) {
    this.listeners.set(name, [...(this.listeners.get(name) || []), fn]);
  }
  emit(name, value = {}) {
    for (const listener of this.listeners.get(name) || []) listener(value);
  }
  open() { this.readyState = 1; this.emit("open"); }
  send(value) { this.sent.push(JSON.parse(value)); }
  reply(id, fields) { this.emit("message", { data: JSON.stringify({ jsonrpc: "2.0", id, ...fields }) }); }
  close() {
    if (this.readyState === 3) return;
    this.readyState = 3;
    this.emit("close");
  }
}

function setup(options = {}) {
  const clock = new Clock();
  const sockets = [];
  const states = [];
  const client = new DashboardRPC("wss://dashboard.example/__encore", {
    setTimer: clock.set, clearTimer: clock.clear,
    makeSocket: () => { const socket = new Socket(); sockets.push(socket); return socket; },
    onState: (state) => states.push(state), ...options,
  });
  client.start();
  return { client, clock, sockets, states };
}

test("empty application results complete successfully rather than remaining pending", async () => {
  const { client, sockets, clock } = setup();
  sockets[0].open();
  const request = client.request("list-apps");
  sockets[0].reply(sockets[0].sent[0].id, { result: [] });
  assert.deepEqual(resultList(await request), []);
  assert.equal(client.pending.size, 0);
  assert.equal(clock.timers.size, 0);
  client.stop();
});

test("RPC rejection reaches the caller and clears its timeout", async () => {
  const { client, sockets, clock } = setup();
  sockets[0].open();
  const rejected = assert.rejects(client.request("status", { appID: "example" }), /unavailable/);
  sockets[0].reply(sockets[0].sent[0].id, { error: { code: -32601, message: "unavailable" } });
  await rejected;
  assert.equal(client.pending.size, 0);
  assert.equal(clock.timers.size, 0);
  client.stop();
});

test("a silent RPC times out and a late response cannot resolve another request", async () => {
  const { client, sockets, clock } = setup({ requestTimeout: 25 });
  sockets[0].open();
  const rejected = assert.rejects(client.request("list-apps"), /timed out/);
  const oldID = sockets[0].sent[0].id;
  clock.advance(25);
  await rejected;
  const fresh = client.request("list-apps");
  sockets[0].reply(oldID, { result: [{ id: "stale" }] });
  assert.equal(client.pending.size, 1);
  sockets[0].reply(sockets[0].sent[1].id, { result: [] });
  assert.deepEqual(await fresh, []);
  client.stop();
});

test("disconnect rejects every pending RPC and reconnects with a fresh socket", async () => {
  const { client, sockets, clock, states } = setup();
  sockets[0].open();
  const first = assert.rejects(client.request("version"), /Connection closed/);
  const second = assert.rejects(client.request("list-apps"), /Connection closed/);
  sockets[0].close();
  await Promise.all([first, second]);
  assert.equal(client.pending.size, 0);
  assert.equal(clock.timers.size, 1, "only the reconnect timer remains");
  assert.equal(states.at(-1).state, "reconnecting");
  clock.advance(1000);
  assert.equal(sockets.length, 2);
  sockets[1].open();
  assert.equal(states.at(-1).state, "connected");
  const request = client.request("list-apps");
  sockets[0].reply(sockets[1].sent[0].id, { result: [{ id: "stale" }] });
  sockets[1].reply(sockets[1].sent[0].id, { result: [] });
  assert.deepEqual(await request, []);
  client.stop();
  assert.equal(clock.timers.size, 0);
});

test("connection attempts have bounded timeouts, backoff, and a final offline state", () => {
  const { client, clock, sockets, states } = setup({ connectTimeout: 5, maxRetries: 2 });
  clock.advance(5);
  assert.equal(states.at(-1).delay, 1000);
  clock.advance(1005);
  assert.equal(states.at(-1).delay, 2000);
  clock.advance(2005);
  assert.equal(sockets.length, 3);
  assert.equal(states.at(-1).state, "offline");
  assert.equal(clock.timers.size, 0);
  client.start();
  sockets.at(-1).open();
  assert.equal(states.at(-1).state, "connected");
  client.stop();
});

test("intentional stop rejects pending work and cancels automatic reconnect", async () => {
  const { client, clock, sockets } = setup();
  sockets[0].open();
  const rejected = assert.rejects(client.request("list-apps"), /Connection closed/);
  client.stop();
  await rejected;
  clock.advance(120000);
  assert.equal(sockets.length, 1);
  assert.equal(client.pending.size, 0);
  assert.equal(clock.timers.size, 0);
});

test("null trace lists are empty while malformed list responses fail explicitly", () => {
  assert.deepEqual(resultList(null, true), []);
  assert.throws(() => resultList(null), /unexpected list/);
  assert.throws(() => resultList({ apps: [] }), /unexpected list/);
});
