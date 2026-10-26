// encore-remote-read-only-ui
// Encore remote read-only dashboard. All data stays on this origin.
import { DashboardRPC, resultList } from "/rpc.js";

const byID = (id) => document.getElementById(id);
const state = {
  apps: null, selected: new URL(location.href).searchParams.get("app"),
  status: null, traces: null, statusError: null, traceError: null,
  connection: "connecting", workspaceError: null,
};
let workspaceGeneration = 0;
let selectionGeneration = 0;
let traceGeneration = 0;
let notificationTimer;

function element(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined && text !== null) node.textContent = String(text);
  if (className) node.className = className;
  return node;
}

function emptyState(title, description, connected = false) {
  const box = element("div", null, "empty-state");
  box.append(element("span", "◇", "empty-symbol"));
  if (connected) box.append(element("p", "YOUR DASHBOARD IS CONNECTED", "empty-kicker"));
  box.append(element("h2", title), element("p", description));
  return box;
}

function notice(message) { return element("p", message, "notice error"); }

function updateWorkspaceError(message) {
  state.workspaceError = message;
  byID("workspace-error").textContent = message || "";
  byID("workspace-error").hidden = !message;
}

function updateConnection({ state: connection, delay }) {
  state.connection = connection;
  const connected = connection === "connected";
  const messages = {
    connected: "Live connection to this workspace.",
    connecting: "Opening a connection to your workspace…",
    reconnecting: `Connection lost. Retrying in ${Math.ceil((delay || 1000) / 1000)}s. Previously loaded data may be stale.`,
    offline: "Unable to connect. Reload to renew your sign-in, or try reconnecting.",
  };
  byID("connection-state").textContent = { connected: "Connected", connecting: "Connecting", reconnecting: "Reconnecting", offline: "Disconnected" }[connection];
  byID("connection-message").textContent = messages[connection];
  byID("connection-dot").className = `status-dot ${connected ? "connected" : connection === "offline" ? "offline" : "pending"}`;
  byID("refresh").textContent = connected ? "Refresh" : "Reconnect";
  byID("refresh").disabled = false;
  if (connected) {
    loadWorkspace();
  } else if (state.apps === null) {
    byID("workspace-content").replaceChildren(emptyState(
      connection === "offline" ? "Your workspace is unavailable" : "Connecting to your workspace",
      connection === "offline" ? "Use Reconnect to try again. If your sign-in has expired, reload this page to sign in again." : messages[connection],
    ));
  }
}

const socketURL = new URL("/__encore", location.href);
socketURL.protocol = location.protocol === "https:" ? "wss:" : "ws:";
const client = new DashboardRPC(socketURL.href, {
  onState: updateConnection,
  onNotification: (message) => {
    if (!/^process\/(start|stop|reload)$/.test(message.method)) return;
    clearTimeout(notificationTimer);
    notificationTimer = setTimeout(() => { if (client.connected) loadWorkspace(); }, 500);
  },
});

async function loadWorkspace() {
  const generation = ++workspaceGeneration;
  byID("refresh").disabled = true;
  updateWorkspaceError(null);
  client.request("version").then((version) => {
    if (generation !== workspaceGeneration) return;
    byID("daemon-version").textContent = typeof version?.version === "string" ? version.version : "Version unavailable";
  }).catch(() => {
    if (generation === workspaceGeneration) byID("daemon-version").textContent = "Version unavailable";
  });
  try {
    const apps = resultList(await client.request("list-apps"));
    if (generation !== workspaceGeneration) return;
    if (apps.some((app) => !app || typeof app.id !== "string" || typeof app.name !== "string")) {
      throw new Error("The server returned an unexpected application response.");
    }
    state.apps = apps;
    if (!state.selected && apps.length) state.selected = apps[0].id;
    renderApps();
    byID("last-updated").textContent = `Updated ${new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`;
    if (apps.some((app) => app.id === state.selected)) await loadSelection();
    else renderSelection();
  } catch (error) {
    if (generation !== workspaceGeneration) return;
    updateWorkspaceError(error.message);
    if (state.apps === null) {
      byID("app-list").replaceChildren(element("p", "Applications could not be loaded.", "muted padded"));
      byID("workspace-content").replaceChildren(emptyState("Applications unavailable", "Refresh to retry. If your sign-in expired, reload this page to sign in again."));
    }
  } finally {
    if (generation === workspaceGeneration) byID("refresh").disabled = false;
  }
}

function renderApps() {
  byID("app-count").textContent = state.apps.length;
  const list = byID("app-list");
  list.replaceChildren();
  if (!state.apps.length) {
    list.append(element("p", "No registered applications", "muted padded"));
    return;
  }
  for (const app of state.apps) {
    const button = element("button", null, "app-choice");
    button.type = "button";
    button.setAttribute("aria-current", String(app.id === state.selected));
    const label = element("span");
    label.append(element("strong", app.name), element("small", app.offline ? "Not running" : "Running"));
    button.append(element("span", null, `status-dot ${app.offline ? "" : "connected"}`), label);
    button.addEventListener("click", () => selectApp(app.id));
    list.append(button);
  }
}

function selectApp(id, updateURL = true) {
  state.selected = id;
  if (updateURL) {
    const url = new URL(location.href);
    url.searchParams.set("app", id);
    history.pushState({}, "", url);
  }
  renderApps();
  loadSelection();
}

async function loadSelection() {
  const generation = ++selectionGeneration;
  traceGeneration += 1;
  state.status = state.traces = null;
  state.statusError = state.traceError = null;
  renderSelection();
  const id = state.selected;
  const statusPromise = client.request("status", { appID: id }).then((status) => {
    if (!status || typeof status.running !== "boolean") throw new Error("The server returned an unexpected application status.");
    if (generation === selectionGeneration) state.status = status;
  }).catch((error) => { if (generation === selectionGeneration) state.statusError = error.message; });
  const tracesPromise = client.request("traces/list", { app_id: id, limit: 50 }).then((traces) => {
    const values = resultList(traces, true);
    if (values.some((trace) => !trace || typeof trace.trace_id !== "string")) throw new Error("The server returned an unexpected trace response.");
    if (generation === selectionGeneration) state.traces = values;
  }).catch((error) => { if (generation === selectionGeneration) state.traceError = error.message; });
  await Promise.all([statusPromise, tracesPromise]);
  if (generation === selectionGeneration) renderSelection();
}

function renderSelection() {
  const content = byID("workspace-content");
  if (!state.apps?.length) {
    content.replaceChildren(emptyState("No applications registered yet", "This Encore daemon has no applications in its workspace. Existing Dokploy services are not discovered automatically; app registration and trace ingestion must be configured separately.", true));
    return;
  }
  const app = state.apps.find((candidate) => candidate.id === state.selected);
  if (!app) {
    content.replaceChildren(emptyState("Application not found", "This application is not registered in the current workspace. Select an application from the list."));
    return;
  }
  const heading = element("div", null, "detail-heading");
  const identity = element("div");
  identity.append(element("h2", app.name), element("p", app.id));
  const status = state.status;
  const badge = status ? (status.running ? "Running" : "Not running") : state.statusError ? "Status unavailable" : "Loading status";
  heading.append(identity, element("span", badge, `badge ${status?.running ? "good" : ""}`));
  content.replaceChildren(heading);
  const services = element("section", null, "detail-section");
  const serviceHeading = element("div", null, "section-heading");
  const serviceList = Array.isArray(status?.meta?.svcs) ? status.meta.svcs : [];
  serviceHeading.append(element("h3", "Services"), element("span", status ? `${serviceList.length} registered` : ""));
  services.append(serviceHeading);
  if (state.statusError) services.append(notice(state.statusError));
  else if (!status) services.append(element("p", "Loading application metadata…", "muted"));
  else if (!serviceList.length) services.append(element("p", "No service metadata is available for this app yet. Metadata is recorded when the app is run with this Encore daemon.", "muted"));
  else {
    const grid = element("div", null, "service-list");
    for (const service of serviceList) {
      const card = element("div", null, "service-card");
      const endpoints = Array.isArray(service.rpcs) ? service.rpcs.length : 0;
      card.append(element("strong", service.name || "Unnamed service"), element("p", `${endpoints} endpoint${endpoints === 1 ? "" : "s"}`));
      grid.append(card);
    }
    services.append(grid);
  }
  content.append(services);
  const traces = element("section", null, "detail-section");
  const traceHeading = element("div", null, "section-heading");
  traceHeading.append(element("h3", "Recent traces"), element("span", "Latest 50 · updates every 30s"));
  traces.append(traceHeading);
  if (state.traceError) traces.append(notice(state.traceError));
  else if (state.traces === null) traces.append(element("p", "Loading traces…", "muted"));
  else if (!state.traces.length) traces.append(element("p", "No traces recorded for this app yet. Requests must reach an app connected to this daemon before traces appear here.", "muted"));
  else traces.append(traceTable(state.traces, (trace) => showTrace(trace, traces)));
  content.append(traces);
}

function duration(nanos) {
  const millis = Number(nanos || 0) / 1e6;
  if (!Number.isFinite(millis)) return "—";
  return millis < 1000 ? `${millis.toFixed(millis < 10 ? 2 : 0)} ms` : `${(millis / 1000).toFixed(2)} s`;
}

function startedAt(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function traceTable(items, onSelect) {
  const wrap = element("div", null, "table-wrap");
  const table = element("table");
  const head = element("thead");
  const headings = element("tr");
  for (const title of ["Request", "Started", "Duration", "Result"]) headings.append(element("th", title));
  head.append(headings);
  const body = element("tbody");
  for (const trace of items) {
    const row = element("tr");
    const title = trace.endpoint_name || trace.topic_name || trace.type || "Span";
    const name = element("td");
    const label = onSelect ? element("button", title, "trace-button") : element("strong", title);
    if (onSelect) {
      label.type = "button";
      label.addEventListener("click", () => onSelect(trace));
    }
    name.append(label, element("span", trace.service_name || "Unknown service", "trace-subtitle"));
    const result = element("td");
    result.append(element("span", trace.is_error ? "Error" : "OK", `badge ${trace.is_error ? "bad" : "good"}`));
    row.append(name, element("td", startedAt(trace.started_at)), element("td", duration(trace.duration_nanos)), result);
    body.append(row);
  }
  table.append(head, body);
  wrap.append(table);
  return wrap;
}

async function showTrace(trace, section) {
  const generation = ++traceGeneration;
  const appID = state.selected;
  section.querySelector(".trace-detail")?.remove();
  const detail = element("div", null, "trace-detail");
  detail.append(element("h3", "Trace spans"), element("p", trace.trace_id, "trace-id"), element("p", "Loading spans…", "muted"));
  section.append(detail);
  try {
    const spans = resultList(await client.request("traces/spans/summaries/list", { app_id: appID, trace_id: trace.trace_id }), true);
    if (generation !== traceGeneration || appID !== state.selected) return;
    detail.replaceChildren(element("h3", "Trace spans"), element("p", trace.trace_id, "trace-id"));
    if (spans.length) detail.append(traceTable(spans, null));
    else detail.append(element("p", "No span summaries are available for this trace.", "muted"));
  } catch (error) {
    if (generation === traceGeneration) detail.replaceChildren(element("h3", "Trace unavailable"), notice(error.message));
  }
}

byID("refresh").addEventListener("click", () => client.connected ? loadWorkspace() : client.start());
window.addEventListener("popstate", () => {
  state.selected = new URL(location.href).searchParams.get("app");
  if (!state.apps) return;
  if (!state.selected && state.apps.length) state.selected = state.apps[0].id;
  renderApps();
  if (state.apps.some((app) => app.id === state.selected)) loadSelection();
  else renderSelection();
});
let pollTimer;
function startPolling() {
  clearInterval(pollTimer);
  pollTimer = setInterval(() => {
    if (client.connected && !document.hidden && !byID("refresh").disabled) loadWorkspace();
  }, 30000);
}
window.addEventListener("pagehide", () => {
  clearInterval(pollTimer);
  clearTimeout(notificationTimer);
  client.stop();
});
window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    startPolling();
    client.start();
  }
});
startPolling();
client.start();
