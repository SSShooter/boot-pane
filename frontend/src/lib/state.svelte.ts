import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
import { api } from "./api";

/** 把 Go 侧返回的 error 转成能直接展示的文案。 */
function describeError(e: unknown): string {
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  if (e && typeof e === "object" && "message" in e) return String((e as any).message);
  return String(e);
}

class AppState {
  projects = $state<ResolvedProject[]>([]);
  runtimes = $state<Record<string, Runtime>>({});
  selectedId = $state("");
  loading = $state(true);

  /** 当前打开「生成配置」弹窗的项目 id，null 表示弹窗关闭。 */
  generatorProjectId = $state<string | null>(null);

  toast = $state("");
  toastIsError = $state(false);
  private toastTimer: ReturnType<typeof setTimeout> | undefined;

  selected = $derived(this.projects.find((p) => p.id === this.selectedId) ?? null);

  generatorProject = $derived(
    this.projects.find((p) => p.id === this.generatorProjectId) ?? null,
  );

  /** 拉取项目列表，并在必要时修正当前选中项。 */
  async load(): Promise<void> {
    this.loading = true;
    try {
      const list = await api.listProjects();
      this.projects = list ?? [];
      if (!this.projects.some((p) => p.id === this.selectedId)) {
        this.selectedId = this.projects[0]?.id ?? "";
      }
      await this.refreshRuntimes();
    } catch (e) {
      this.notify(describeError(e), true);
    } finally {
      this.loading = false;
    }
  }

  /** 同步任务运行状态。M1 之后会改成事件推送，这里先做一次性拉取。 */
  async refreshRuntimes(): Promise<void> {
    try {
      const list = await api.listRunning();
      const next: Record<string, Runtime> = {};
      for (const r of list ?? []) {
        next[`${r.projectId}/${r.taskId}`] = r;
      }
      this.runtimes = next;
    } catch {
      // 进程服务尚未接入时不打扰用户
    }
  }

  select(id: string): void {
    this.selectedId = id;
  }

  runtimeOf(projectId: string, taskId: string): Runtime | null {
    return this.runtimes[`${projectId}/${taskId}`] ?? null;
  }

  async addProject(path: string): Promise<void> {
    if (!path) return;
    try {
      const added = await api.addProject(path);
      await this.load();
      this.selectedId = added.id;
      this.notify(`已添加 ${added.name}`);
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async removeProject(id: string): Promise<void> {
    try {
      await api.removeProject(id);
      if (this.selectedId === id) this.selectedId = "";
      await this.load();
      this.notify("已从列表移除（磁盘文件未受影响）");
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async openProjectDir(id: string): Promise<void> {
    try {
      await api.openProjectDir(id);
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async openConfigDir(): Promise<void> {
    try {
      await api.openConfigDir();
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  /** 切换配置来源：全局注册表 ↔ 仓库内 .bootpane.json。 */
  async setUseRepoConfig(id: string, enabled: boolean): Promise<void> {
    try {
      await api.setUseRepoConfig(id, enabled);
      await this.load();
      this.notify(enabled ? "已切换为项目内配置" : "已切换为全局配置");
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  openGenerator(id: string): void {
    this.generatorProjectId = id;
  }

  closeGenerator(): void {
    this.generatorProjectId = null;
  }

  async startTask(projectId: string, taskId: string): Promise<void> {
    try {
      await api.startTask(projectId, taskId);
      await this.refreshRuntimes();
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async stopTask(projectId: string, taskId: string): Promise<void> {
    try {
      await api.stopTask(projectId, taskId);
      await this.refreshRuntimes();
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  notify(message: string, isError = false): void {
    this.toast = message;
    this.toastIsError = isError;
    clearTimeout(this.toastTimer);
    this.toastTimer = setTimeout(
      () => {
        this.toast = "";
      },
      isError ? 7000 : 2600,
    );
  }
}

export const app = new AppState();
