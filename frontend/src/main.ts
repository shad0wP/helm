// Helm frontend — typed vanilla TS over the generated Wails v3 bindings.
// Bundle the Tabler icon webfont locally (no runtime CDN dependency).
import "@tabler/icons-webfont/dist/tabler-icons.min.css";
import { App } from "../bindings/helm";
import { ServiceKind, type Service } from "../bindings/helm/internal/service";
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

let toastTimer: ReturnType<typeof setTimeout> | undefined;

// showToast surfaces a non-blocking status/error message. Text is set via
// textContent only (never innerHTML), so backend messages can't inject markup.
function showToast(message: string, kind: "info" | "error" = "info"): void {
  const toast = byId("toast");
  toast.textContent = message;
  toast.className = "toast " + kind;
  toast.hidden = false;
  if (toastTimer !== undefined) {
    clearTimeout(toastTimer);
  }
  toastTimer = setTimeout(() => {
    toast.hidden = true;
  }, kind === "error" ? 6000 : 3000);
}

// errText extracts a human-readable string from an unknown thrown value.
function errText(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  if (typeof err === "string") {
    return err;
  }
  return String(err);
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

  // Only pure KindPort probes are read-only. KindProcess is stoppable (via
  // PID/supervisor resolution), so its toggle is live.
  const readonly: boolean = svc.Kind === ServiceKind.KindPort;
  const toggle = document.createElement("label");
  toggle.className = "toggle" + (readonly ? " readonly" : "");
  toggle.title = readonly ? "Read-only — declare it in ~/.config/helm/services.json to control it" : "";

  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = svc.Running;
  input.disabled = readonly;
  if (!readonly) {
    input.addEventListener("change", () => {
      void toggleService(svc.ID, svc.Name, input);
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
      : svc.Kind === ServiceKind.KindProcess
      ? "process"
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

async function toggleService(id: string, name: string, checkbox: HTMLInputElement): Promise<void> {
  const wanted = checkbox.checked;
  try {
    await App.Toggle(id);
  } catch (err: unknown) {
    // Revert the optimistic toggle AND surface the real failure — a silently
    // reverted checkbox otherwise looks like "nothing happened".
    checkbox.checked = !wanted;
    showToast(`${name}: ${errText(err)}`, "error");
    console.error("Toggle failed:", err);
  }
}

function wireControls(): void {
  byId<HTMLButtonElement>("btn-start-all").addEventListener("click", () => {
    App.StartAll().catch((err: unknown) => showToast(`Start all: ${errText(err)}`, "error"));
  });
  byId<HTMLButtonElement>("btn-stop-all").addEventListener("click", () => {
    App.StopAll().catch((err: unknown) => showToast(`Stop all: ${errText(err)}`, "error"));
  });
  byId<HTMLButtonElement>("btn-close").addEventListener("click", () => {
    App.HideWindow().catch((err: unknown) => console.error("Hide failed:", err));
  });

  const vramBtn = byId<HTMLButtonElement>("btn-vram");
  vramBtn.addEventListener("click", async (): Promise<void> => {
    vramBtn.disabled = true;
    try {
      const n = await App.FreeVRAM();
      showToast(n > 0 ? `Freed VRAM — unloaded ${n} model(s)` : "No models were loaded", "info");
    } catch (err: unknown) {
      showToast(`Free VRAM: ${errText(err)}`, "error");
    } finally {
      vramBtn.disabled = false;
    }
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
