// Helm frontend — typed vanilla TS over the generated Wails v3 bindings.
// Bundle the Tabler icon webfont locally (no runtime CDN dependency).
import "@tabler/icons-webfont/dist/tabler-icons.min.css";
import { App } from "../bindings/helm";
import { ServiceKind, type Service } from "../bindings/helm/internal/service";
import { type Info as UpdateInfo } from "../bindings/helm/internal/update";
import { Events } from "@wailsio/runtime";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isService(value: unknown): value is Service {
  return (
    isRecord(value) &&
    typeof value.ID === "string" &&
    typeof value.Name === "string" &&
    typeof value.Kind === "string" &&
    typeof value.Unit === "string" &&
    typeof value.Container === "string" &&
    typeof value.Port === "number" &&
    typeof value.Running === "boolean" &&
    typeof value.Icon === "string" &&
    typeof value.Color === "string" &&
    typeof value.Meta === "string" &&
    typeof value.Auto === "boolean"
  );
}

function serviceSnapshot(value: unknown): Service[] | null {
  return Array.isArray(value) && value.every(isService) ? value : null;
}

function isUpdateInfo(value: unknown): value is UpdateInfo {
  return (
    isRecord(value) &&
    typeof value.currentVersion === "string" &&
    typeof value.latestVersion === "string" &&
    typeof value.updateAvailable === "boolean" &&
    typeof value.releaseURL === "string" &&
    typeof value.releaseNotes === "string" &&
    typeof value.assetURL === "string" &&
    typeof value.checksumsURL === "string"
  );
}

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
  wireUpdateControls();
  void showVersion();
  render((await App.GetServices()) ?? []);

  // Backend pushes a fresh snapshot after every poll cycle that changes state.
  Events.On("services-updated", (ev): void => {
    const snapshot = serviceSnapshot(ev.data);
    if (snapshot === null) {
      console.error("Helm: rejected malformed services-updated event", ev.data);
      return;
    }
    render(snapshot);
  });

  // Background update checker pushes this when a newer release is found.
  Events.On("update-available", (ev): void => {
    if (!isUpdateInfo(ev.data)) {
      console.error("Helm: rejected malformed update-available event", ev.data);
      return;
    }
    showUpdateBanner(ev.data);
  });
}

async function showVersion(): Promise<void> {
  try {
    byId("app-version").textContent = "v" + (await App.GetVersion());
  } catch (err: unknown) {
    console.error("version:", err);
  }
}

// showUpdateBanner renders the dismissible update banner. All text via
// textContent; the release URL / asset URL are captured in closures, never
// interpolated into markup.
function showUpdateBanner(info: UpdateInfo): void {
  if (!info.updateAvailable) {
    return;
  }
  const banner = byId("update-banner");
  byId("update-text").textContent = `Helm ${info.latestVersion} is available (you have v${info.currentVersion})`;

  const view = byId<HTMLButtonElement>("update-view");
  view.hidden = !info.releaseURL;
  view.onclick = () => {
    App.OpenReleasePage(info.releaseURL).catch((e: unknown) => showToast(errText(e), "error"));
  };

  const dl = byId<HTMLButtonElement>("update-download");
  dl.hidden = !info.assetURL;
  dl.onclick = async () => {
    dl.disabled = true;
    try {
      const path = await App.DownloadUpdate(info.assetURL);
      showToast(`Downloaded & verified → ${path}`, "info");
    } catch (err: unknown) {
      showToast(`Download: ${errText(err)}`, "error");
    } finally {
      dl.disabled = false;
    }
  };

  byId<HTMLButtonElement>("update-dismiss").onclick = () => {
    banner.hidden = true;
  };
  banner.hidden = false;
}

function wireUpdateControls(): void {
  const checkBtn = byId<HTMLButtonElement>("btn-check-update");
  checkBtn.addEventListener("click", async (): Promise<void> => {
    checkBtn.disabled = true;
    try {
      const info = await App.CheckForUpdate();
      if (info.updateAvailable) {
        showUpdateBanner(info);
      } else {
        showToast("You're up to date", "info");
      }
    } catch (err: unknown) {
      showToast(`Update check: ${errText(err)}`, "error");
    } finally {
      checkBtn.disabled = false;
    }
  });
}

function render(services: readonly Service[]): void {
  updateGlobalStatus(services);
  const list = byId("services-list");
  const fragment = document.createDocumentFragment();
  const known: Service[] = [];
  const auto: Service[] = [];
  for (const service of services) {
    (service.Auto ? auto : known).push(service);
  }

  if (known.length) {
    appendSection(fragment, "Services", known);
  }
  if (auto.length) {
    const div = document.createElement("div");
    div.className = "divider";
    fragment.appendChild(div);
    appendSection(fragment, "Auto-detected", auto);
  }
  if (!services.length) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "No services found";
    fragment.appendChild(empty);
  }
  list.replaceChildren(fragment);
}

function appendSection(parent: ParentNode, label: string, services: readonly Service[]): void {
  const lbl = document.createElement("div");
  lbl.className = "section-label";
  lbl.textContent = label;
  parent.appendChild(lbl);
  for (const service of services) {
    parent.appendChild(buildRow(service));
  }
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

  // Raw processes are stop-only: Helm can signal a running process but cannot
  // reconstruct the command needed to launch a stopped one.
  const readonly = svc.Kind === ServiceKind.KindPort || (svc.Kind === ServiceKind.KindProcess && !svc.Running);
  const toggle = document.createElement("label");
  toggle.className = "toggle" + (readonly ? " readonly" : "");
  if (svc.Kind === ServiceKind.KindProcess && !svc.Running) {
    toggle.title = "Cannot start — launch this service externally";
  } else if (readonly) {
    toggle.title = "Read-only — declare it in ~/.config/helm/services.json to control it";
  }

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
  let proto: string;
  switch (svc.Kind) {
    case ServiceKind.KindDocker:
      proto = "docker";
      break;
    case ServiceKind.KindSystemctl:
      proto = "systemd";
      break;
    case ServiceKind.KindProcess:
      proto = "process";
      break;
    default:
      proto = "port";
  }
  return svc.Port ? `${proto} · localhost:${svc.Port}` : proto;
}

function updateGlobalStatus(services: readonly Service[]): void {
  let running = 0;
  for (const service of services) {
    if (service.Running) {
      running++;
    }
  }
  const total = services.length;
  const dot = byId("global-dot");
  const txt = byId("global-text");

  let state = "state-none";
  if (total > 0 && running === total) {
    state = "state-all";
  } else if (running > 0) {
    state = "state-some";
  }
  dot.className = `status-dot ${state}`;

  if (total === 0) {
    txt.textContent = "No services found";
  } else if (running === 0) {
    txt.textContent = "All stopped";
  } else {
    txt.textContent = `${running} of ${total} running`;
  }
}

async function toggleService(id: string, name: string, checkbox: HTMLInputElement): Promise<void> {
  const wanted = checkbox.checked;
  checkbox.disabled = true;
  try {
    render((await App.Toggle(id)) ?? []);
  } catch (err: unknown) {
    // Revert the optimistic toggle AND surface the real failure — a silently
    // reverted checkbox otherwise looks like "nothing happened".
    checkbox.checked = !wanted;
    showToast(`${name}: ${errText(err)}`, "error");
    console.error("Toggle failed:", err);
  } finally {
    checkbox.disabled = false;
  }
}

function wireControls(): void {
  const startAll = byId<HTMLButtonElement>("btn-start-all");
  startAll.addEventListener("click", () => {
    void runBulkAction(startAll, "Start all", App.StartAll);
  });
  const stopAll = byId<HTMLButtonElement>("btn-stop-all");
  stopAll.addEventListener("click", () => {
    void runBulkAction(stopAll, "Stop all", App.StopAll);
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
    setButtonContent(scanBtn, "ti-loader-2 ti-spin", "Scanning…");
    try {
      render((await App.Scan()) ?? []);
    } catch (err: unknown) {
      console.error("Scan failed:", err);
    } finally {
      scanBtn.disabled = false;
      setButtonContent(scanBtn, "ti-radar", "Scan for services");
    }
  });
}

async function runBulkAction(
  button: HTMLButtonElement,
  label: string,
  action: () => Promise<Service[] | null>,
): Promise<void> {
  button.disabled = true;
  try {
    render((await action()) ?? []);
  } catch (err: unknown) {
    showToast(`${label}: ${errText(err)}`, "error");
  } finally {
    button.disabled = false;
  }
}

function setButtonContent(button: HTMLButtonElement, iconClass: string, text: string): void {
  button.replaceChildren();
  const icon = document.createElement("i");
  icon.className = `ti ${iconClass}`;
  icon.setAttribute("aria-hidden", "true");
  button.append(icon, document.createTextNode(` ${text}`));
}

init().catch((err: unknown) => console.error("Helm init failed:", err));
