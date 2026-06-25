// Helm frontend — vanilla JS over the generated Wails v3 bindings.
import { App } from "../bindings/helm";
import { Events } from "@wailsio/runtime";

let currentServices = [];

async function init() {
  wireControls();
  currentServices = (await App.GetServices()) || [];
  render(currentServices);

  // Backend pushes a fresh snapshot after every poll cycle that changes state.
  Events.On("services-updated", (e) => {
    currentServices = e.data || [];
    render(currentServices);
  });
}

function render(services) {
  updateGlobalStatus(services);
  const list = document.getElementById("services-list");
  list.innerHTML = "";

  const known = services.filter((s) => !s.Auto);
  const auto = services.filter((s) => s.Auto);

  if (known.length) {
    appendSection(list, "Services", known);
  }
  if (auto.length) {
    const div = document.createElement("div");
    div.className = "divider";
    list.appendChild(div);
    appendSection(list, "Auto-detected", auto);
  }
  if (!services.length) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "No services found";
    list.appendChild(empty);
  }
}

function appendSection(parent, label, services) {
  const lbl = document.createElement("div");
  lbl.className = "section-label";
  lbl.textContent = label;
  parent.appendChild(lbl);
  services.forEach((svc) => parent.appendChild(buildRow(svc)));
}

function buildRow(svc) {
  const row = document.createElement("div");
  row.className = "service-row";
  row.id = "row-" + svc.ID;

  const icon = document.createElement("div");
  icon.className = `svc-icon ${svc.Color}`;
  icon.innerHTML = `<i class="ti ti-${svc.Icon}" aria-hidden="true"></i>`;

  const info = document.createElement("div");
  info.className = "svc-info";
  const name = document.createElement("div");
  name.className = "svc-name";
  name.textContent = svc.Name;
  const meta = document.createElement("div");
  meta.className = "svc-meta";
  meta.textContent = svc.Meta || portLabel(svc);
  info.append(name, meta);

  const isPort = svc.Kind === "port";
  const toggle = document.createElement("label");
  toggle.className = "toggle" + (isPort ? " readonly" : "");
  toggle.title = isPort ? "Cannot control — started externally" : "";

  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = !!svc.Running;
  input.disabled = isPort;
  if (!isPort) {
    input.addEventListener("change", () => toggleService(svc.ID, input));
  }

  const track = document.createElement("div");
  track.className = "toggle-track";

  toggle.append(input, track);
  row.append(icon, info, toggle);
  return row;
}

function portLabel(svc) {
  const proto =
    svc.Kind === "docker" ? "docker" : svc.Kind === "systemctl" ? "systemd" : "port";
  return svc.Port ? `${proto} · localhost:${svc.Port}` : proto;
}

function updateGlobalStatus(services) {
  const running = services.filter((s) => s.Running).length;
  const total = services.length;
  const dot = document.getElementById("global-dot");
  const txt = document.getElementById("global-text");

  dot.className =
    "status-dot " +
    (running === total && total > 0
      ? "state-all"
      : running > 0
      ? "state-some"
      : "state-none");

  txt.textContent =
    total === 0
      ? "No services found"
      : running === 0
      ? "All stopped"
      : `${running} of ${total} running`;
}

async function toggleService(id, checkbox) {
  try {
    await App.Toggle(id);
  } catch (e) {
    // Revert the optimistic toggle on error.
    checkbox.checked = !checkbox.checked;
    console.error("Toggle failed:", e);
  }
}

function wireControls() {
  document
    .getElementById("btn-start-all")
    .addEventListener("click", () => App.StartAll().catch(console.error));
  document
    .getElementById("btn-stop-all")
    .addEventListener("click", () => App.StopAll().catch(console.error));
  document
    .getElementById("btn-close")
    .addEventListener("click", () => App.HideWindow());

  const scanBtn = document.getElementById("btn-scan");
  scanBtn.addEventListener("click", async () => {
    scanBtn.disabled = true;
    scanBtn.innerHTML =
      '<i class="ti ti-loader-2 ti-spin" aria-hidden="true"></i> Scanning…';
    try {
      await App.Scan();
    } catch (e) {
      console.error("Scan failed:", e);
    }
    scanBtn.disabled = false;
    scanBtn.innerHTML =
      '<i class="ti ti-radar" aria-hidden="true"></i> Scan for services';
  });
}

init();
