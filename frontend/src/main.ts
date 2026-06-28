// Helm frontend — typed vanilla TS over the generated Wails v3 bindings.
// Bundle the Tabler icon webfont locally (no runtime CDN dependency).
import "@tabler/icons-webfont/dist/tabler-icons.min.css";
import { App, ServiceKind, type Service } from "../bindings/helm";
import { Events } from "@wailsio/runtime";

let currentServices: Service[] = [];

// byId returns a required element typed as T, throwing if the markup is missing it.
function byId<T extends HTMLElement = HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (node === null) {
    throw new Error(`Helm: required element #${id} not found`);
  }
  return node as T;
}

// cssToken sanitises a value before it is used in a class name, allowing only
// CSS-identifier-safe characters. Defence-in-depth at the rendering boundary so
// service-derived strings can never carry markup or break out of an attribute.
function cssToken(value: string): string {
  return value.replace(/[^a-z0-9-]/gi, "");
}

async function init(): Promise<void> {
  wireControls();
  currentServices = (await App.GetServices()) ?? [];
  render(currentServices);

  // Backend pushes a fresh snapshot after every poll cycle that changes state.
  Events.On("services-updated", (ev): void => {
    currentServices = (ev.data as Service[] | null) ?? [];
    render(currentServices);
  });
}

function render(services: Service[]): void {
  updateGlobalStatus(services);
  const list = byId("services-list");
  list.innerHTML = "";

  const known: Service[] = services.filter((s: Service) => !s.Auto);
  const auto: Service[] = services.filter((s: Service) => s.Auto);

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

function appendSection(parent: HTMLElement, label: string, services: Service[]): void {
  const lbl = document.createElement("div");
  lbl.className = "section-label";
  lbl.textContent = label;
  parent.appendChild(lbl);
  services.forEach((svc: Service) => parent.appendChild(buildRow(svc)));
}

function buildRow(svc: Service): HTMLDivElement {
  const row = document.createElement("div");
  row.className = "service-row";
  row.id = "row-" + svc.ID;

  const icon = document.createElement("div");
  icon.className = `svc-icon ${cssToken(svc.Color)}`;
  // Build the glyph via the DOM API (no innerHTML): the icon name flows into a
  // class name, never into parsed HTML, so it cannot inject markup/script.
  const glyph = document.createElement("i");
  glyph.className = `ti ti-${cssToken(svc.Icon)}`;
  glyph.setAttribute("aria-hidden", "true");
  icon.appendChild(glyph);

  const info = document.createElement("div");
  info.className = "svc-info";
  const name = document.createElement("div");
  name.className = "svc-name";
  name.textContent = svc.Name;
  const meta = document.createElement("div");
  meta.className = "svc-meta";
  meta.textContent = svc.Meta || portLabel(svc);
  info.append(name, meta);

  const isPort: boolean = svc.Kind === ServiceKind.KindPort;
  const toggle = document.createElement("label");
  toggle.className = "toggle" + (isPort ? " readonly" : "");
  toggle.title = isPort ? "Cannot control — started externally" : "";

  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = svc.Running;
  input.disabled = isPort;
  if (!isPort) {
    input.addEventListener("change", () => {
      void toggleService(svc.ID, input);
    });
  }

  const track = document.createElement("div");
  track.className = "toggle-track";

  toggle.append(input, track);
  row.append(icon, info, toggle);
  return row;
}

function portLabel(svc: Service): string {
  const proto: string =
    svc.Kind === ServiceKind.KindDocker
      ? "docker"
      : svc.Kind === ServiceKind.KindSystemctl
      ? "systemd"
      : "port";
  return svc.Port ? `${proto} · localhost:${svc.Port}` : proto;
}

function updateGlobalStatus(services: Service[]): void {
  const running: number = services.filter((s: Service) => s.Running).length;
  const total: number = services.length;
  const dot = byId("global-dot");
  const txt = byId("global-text");

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

async function toggleService(id: string, checkbox: HTMLInputElement): Promise<void> {
  try {
    await App.Toggle(id);
  } catch (err: unknown) {
    // Revert the optimistic toggle on error.
    checkbox.checked = !checkbox.checked;
    console.error("Toggle failed:", err);
  }
}

function wireControls(): void {
  byId<HTMLButtonElement>("btn-start-all").addEventListener("click", () => {
    App.StartAll().catch((err: unknown) => console.error("Start all failed:", err));
  });
  byId<HTMLButtonElement>("btn-stop-all").addEventListener("click", () => {
    App.StopAll().catch((err: unknown) => console.error("Stop all failed:", err));
  });
  byId<HTMLButtonElement>("btn-close").addEventListener("click", () => {
    App.HideWindow().catch((err: unknown) => console.error("Hide failed:", err));
  });

  const scanBtn = byId<HTMLButtonElement>("btn-scan");
  scanBtn.addEventListener("click", async (): Promise<void> => {
    scanBtn.disabled = true;
    scanBtn.innerHTML =
      '<i class="ti ti-loader-2 ti-spin" aria-hidden="true"></i> Scanning…';
    try {
      await App.Scan();
    } catch (err: unknown) {
      console.error("Scan failed:", err);
    }
    scanBtn.disabled = false;
    scanBtn.innerHTML =
      '<i class="ti ti-radar" aria-hidden="true"></i> Scan for services';
  });
}

init().catch((err: unknown) => console.error("Helm init failed:", err));
