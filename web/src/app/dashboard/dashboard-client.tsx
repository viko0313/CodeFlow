"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Database, FolderGit2, Gauge, PlugZap, Plus, Save, Server, Sparkles } from "lucide-react";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  createSession,
  getApprovals,
  getConfig,
  getHealth,
  getMcp,
  getRecentAudit,
  getSessions,
  getSkills,
  getTaskEvents,
  updateModelConfig,
} from "@/lib/codeflow-api";
import { formatDateTime } from "@/lib/utils";

type ModelForm = {
  provider: string;
  model: string;
  base_url: string;
  api_key: string;
};

export function DashboardClient({ userName }: { userName: string }) {
  const queryClient = useQueryClient();
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth, refetchInterval: 5000 });
  const config = useQuery({ queryKey: ["config"], queryFn: getConfig });
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: getSessions });
  const skills = useQuery({ queryKey: ["skills"], queryFn: getSkills });
  const mcp = useQuery({ queryKey: ["mcp"], queryFn: getMcp });
  const audit = useQuery({ queryKey: ["audit"], queryFn: getRecentAudit, refetchInterval: 7000 });
  const pendingApprovals = useQuery({
    queryKey: ["approvals", "pending", "dashboard"],
    queryFn: () => getApprovals({ status: "pending", limit: 50 }),
    refetchInterval: 4000,
  });
  const taskEvents = useQuery({
    queryKey: ["task-events", "dashboard"],
    queryFn: () => getTaskEvents({ limit: 60 }),
    refetchInterval: 7000,
  });

  const [modelForm, setModelForm] = useState<ModelForm>({
    provider: "",
    model: "",
    base_url: "",
    api_key: "",
  });

  useEffect(() => {
    if (!config.data) return;
    setModelForm((current) => ({
      provider: config.data.provider ?? "",
      model: config.data.model ?? "",
      base_url: config.data.base_url ?? "",
      api_key: current.api_key,
    }));
  }, [config.data]);

  const create = useMutation({
    mutationFn: () => createSession(`会话 ${new Date().toLocaleTimeString()}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["sessions"] }),
  });

  const saveModel = useMutation({
    mutationFn: () => updateModelConfig(modelForm),
    onSuccess: (updated) => {
      setModelForm({
        provider: updated.provider,
        model: updated.model,
        base_url: updated.base_url,
        api_key: "",
      });
      queryClient.invalidateQueries({ queryKey: ["config"] });
      queryClient.invalidateQueries({ queryKey: ["health"] });
    },
  });

  const skillItems = skills.data?.skills ?? [];
  const mcpServers = mcp.data?.servers ?? [];
  const metrics = (audit.data ?? []).slice(0, 8).reverse().map((event, index) => ({
    name: `${index + 1}`,
    duration: event.duration_ms ?? 0,
  }));

  function updateModelForm(field: keyof ModelForm, value: string) {
    setModelForm((current) => ({ ...current, [field]: value }));
  }

  function submitModelConfig(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    saveModel.mutate();
  }

  return (
    <div className="mx-auto grid max-w-[1480px] gap-5 px-4 py-5">
      <section className="grid gap-4 rounded-lg border border-[var(--line)] bg-white p-5 md:grid-cols-[1.4fr_1fr]">
        <div>
          <Badge>本地工作台</Badge>
          <h1 className="mt-3 text-3xl font-semibold">欢迎回来，{userName}。</h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">
            本地 CodeFlow 服务已经准备好项目会话、运行健康状态、审批数据和最近执行审计。
          </p>
          <div className="mt-5 flex flex-wrap gap-2">
            <Button asChild>
              <Link href="/ide">打开 IDE</Link>
            </Button>
            <Button variant="secondary" onClick={() => create.mutate()} disabled={create.isPending}>
              <Plus className="h-4 w-4" />
              新建会话
            </Button>
          </div>
        </div>
        <div className="grid min-h-44 place-items-center rounded-lg bg-[var(--panel-strong)] p-5">
          <div className="grid w-full grid-cols-3 gap-3 text-center">
            <Metric icon={<Server />} label="接口" value={health.data?.status ?? "检查中"} />
            <Metric icon={<Database />} label="存储" value={health.data?.storage_backend ?? "未知"} />
            <Metric icon={<Gauge />} label="模型" value={health.data?.model_configured ? "已配置" : "未配置"} />
          </div>
        </div>
      </section>

      <section className="grid gap-5 lg:grid-cols-[1.15fr_0.85fr]">
        <div className="rounded-lg border border-[var(--line)] bg-white p-5">
          <div className="mb-4 flex items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold">会话</h2>
              <p className="text-sm text-[var(--muted)]">需要切换到其他线程时，可以在这里直接进入对应会话。</p>
            </div>
            <Badge>共 {sessions.data?.length ?? 0} 个</Badge>
          </div>
          <div className="grid gap-3">
            {(sessions.data ?? []).map((session) => (
              <Link
                key={session.id}
                href={`/ide?session=${session.id}`}
                className="grid gap-2 rounded-lg border border-[var(--line)] p-4 hover:border-[var(--accent)]"
              >
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium">{session.title}</span>
                  {session.active ? <Badge className="border-[var(--accent)] text-[var(--accent-strong)]">当前</Badge> : null}
                </div>
                <span className="truncate text-xs text-[var(--muted)]">{session.id}</span>
                <span className="text-xs text-[var(--muted)]">更新时间 {formatDateTime(session.updated_at)}</span>
              </Link>
            ))}
            {!sessions.data?.length ? (
              <p className="rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
                还没有会话，点击上方按钮开始创建。
              </p>
            ) : null}
          </div>
        </div>

        <div className="grid gap-5">
          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-lg font-semibold">运行环境</h2>
              <Badge>{config.data?.api_key_configured ? config.data.api_key_hint || "密钥已保存" : "未保存密钥"}</Badge>
            </div>
            <dl className="mt-4 grid gap-3 text-sm">
              <Row label="提供方" value={config.data?.provider ?? "未知"} />
              <Row label="模型" value={config.data?.model ?? "未知"} />
              <Row label="Agent" value={config.data?.agent?.mode ?? "react"} />
              <Row label="计划模式" value={config.data?.agent?.plan_enabled ? "已启用" : "请在 IDE 中切换"} />
              <Row label="项目" value={health.data?.project_root ?? config.data?.project_root ?? "未知"} />
              <Row label="记忆" value={health.data?.memory_backend ?? "未知"} />
              <Row label="兜底" value={health.data?.fallback_active ? "已开启" : "关闭"} />
            </dl>
            <form className="mt-5 grid gap-3 border-t border-[var(--line)] pt-4" onSubmit={submitModelConfig}>
              <div className="grid gap-3 sm:grid-cols-2">
                <Input
                  aria-label="提供方"
                  placeholder="provider"
                  value={modelForm.provider}
                  onChange={(event) => updateModelForm("provider", event.target.value)}
                  required
                />
                <Input
                  aria-label="模型"
                  placeholder="model"
                  value={modelForm.model}
                  onChange={(event) => updateModelForm("model", event.target.value)}
                  required
                />
              </div>
              <Input
                aria-label="Base URL"
                placeholder="base_url"
                value={modelForm.base_url}
                onChange={(event) => updateModelForm("base_url", event.target.value)}
              />
              <Input
                aria-label="API Key"
                placeholder="API key 留空则保留现有密钥"
                type="password"
                value={modelForm.api_key}
                onChange={(event) => updateModelForm("api_key", event.target.value)}
              />
              {saveModel.error ? <p className="text-sm text-red-600">{(saveModel.error as Error).message}</p> : null}
              <Button type="submit" size="sm" disabled={saveModel.isPending}>
                <Save className="h-4 w-4" />
                保存模型配置
              </Button>
            </form>
          </div>

          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <h2 className="text-lg font-semibold">审批</h2>
            <p className="mt-2 text-sm text-[var(--muted)]">待处理：{pendingApprovals.data?.length ?? 0}</p>
            <p className="mt-1 text-sm text-[var(--muted)]">
              最近审批事件：{(taskEvents.data ?? []).filter((item) => item.event_type.startsWith("approval.")).length}
            </p>
            <div className="mt-3">
              <Button asChild size="sm" variant="secondary">
                <Link href="/approvals">打开审批中心</Link>
              </Button>
            </div>
          </div>

          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="mb-3 flex items-center gap-2">
              <Sparkles className="h-4 w-4 text-[var(--accent-strong)]" />
              <h2 className="text-lg font-semibold">技能</h2>
            </div>
            <p className="text-sm text-[var(--muted)]">
              已发现 {skillItems.length} 个，其中预加载 {skillItems.filter((skill) => skill.preloaded).length} 个。
            </p>
          </div>

          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="mb-3 flex items-center gap-2">
              <PlugZap className="h-4 w-4 text-[var(--accent-strong)]" />
              <h2 className="text-lg font-semibold">MCP</h2>
            </div>
            <p className="text-sm text-[var(--muted)]">
              已配置 {mcpServers.length} 个，其中预加载 {mcpServers.filter((server) => server.preloaded).length} 个。
            </p>
          </div>

          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="mb-3 flex items-center gap-2">
              <Activity className="h-4 w-4 text-[var(--accent-strong)]" />
              <h2 className="text-lg font-semibold">操作耗时</h2>
            </div>
            <div className="h-44">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={metrics}>
                  <CartesianGrid stroke="#d8dee3" strokeDasharray="3 3" />
                  <XAxis dataKey="name" tick={{ fontSize: 12 }} />
                  <YAxis tick={{ fontSize: 12 }} />
                  <Tooltip />
                  <Area type="monotone" dataKey="duration" stroke="#0f9f8f" fill="#b7e5df" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>
      </section>

      <section className="rounded-lg border border-[var(--line)] bg-white p-5">
        <div className="mb-4 flex items-center gap-2">
          <FolderGit2 className="h-4 w-4 text-[var(--accent-strong)]" />
          <h2 className="text-lg font-semibold">最近审计</h2>
        </div>
        <div className="grid gap-2">
          {(audit.data ?? []).map((event, index) => (
            <div key={`${event.time}-${index}`} className="grid gap-1 rounded-lg border border-[var(--line)] p-3 md:grid-cols-[160px_1fr_120px]">
              <span className="text-xs text-[var(--muted)]">{formatDateTime(event.time)}</span>
              <span className="truncate text-sm">{event.args_summary || event.event}</span>
              <span className="text-xs text-[var(--muted)]">{event.confirmed === false ? "已拒绝" : event.event}</span>
            </div>
          ))}
          {!audit.data?.length ? <p className="text-sm text-[var(--muted)]">暂时还没有审计事件。</p> : null}
        </div>
      </section>
    </div>
  );
}

function Metric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="rounded-lg bg-white p-3">
      <div className="mx-auto mb-2 grid h-8 w-8 place-items-center text-[var(--accent-strong)] [&_svg]:h-5 [&_svg]:w-5">
        {icon}
      </div>
      <p className="text-xs text-[var(--muted)]">{label}</p>
      <p className="mt-1 truncate text-sm font-semibold">{value}</p>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[110px_1fr] gap-3">
      <dt className="text-[var(--muted)]">{label}</dt>
      <dd className="truncate font-medium">{value}</dd>
    </div>
  );
}
