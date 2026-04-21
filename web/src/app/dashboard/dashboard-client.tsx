"use client";

import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Database, FolderGit2, Gauge, PlugZap, Plus, Server, Sparkles } from "lucide-react";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  createSession,
  getConfig,
  getHealth,
  getMcp,
  getRecentAudit,
  getSessions,
  getSkills,
} from "@/lib/codeflow-api";
import { formatDateTime } from "@/lib/utils";

export function DashboardClient({ userName }: { userName: string }) {
  const queryClient = useQueryClient();
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth, refetchInterval: 5000 });
  const config = useQuery({ queryKey: ["config"], queryFn: getConfig });
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: getSessions });
  const skills = useQuery({ queryKey: ["skills"], queryFn: getSkills });
  const mcp = useQuery({ queryKey: ["mcp"], queryFn: getMcp });
  const audit = useQuery({ queryKey: ["audit"], queryFn: getRecentAudit, refetchInterval: 7000 });
  const create = useMutation({
    mutationFn: () => createSession(`CodeFlow ${new Date().toLocaleTimeString()}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["sessions"] }),
  });
  const metrics = (audit.data ?? []).slice(0, 8).reverse().map((event, index) => ({
    name: `${index + 1}`,
    duration: event.duration_ms ?? 0,
  }));

  return (
    <div className="mx-auto grid max-w-[1480px] gap-5 px-4 py-5">
      <section className="grid gap-4 rounded-lg border border-[var(--line)] bg-white p-5 md:grid-cols-[1.4fr_1fr]">
        <div>
          <Badge>Local workspace</Badge>
          <h1 className="mt-3 text-3xl font-semibold">Good to see you, {userName}.</h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">
            Project sessions, runtime health, approvals, and recent execution telemetry are ready
            from the local CodeFlow server.
          </p>
          <div className="mt-5 flex flex-wrap gap-2">
            <Button asChild>
              <Link href="/ide">Open IDE</Link>
            </Button>
            <Button variant="secondary" onClick={() => create.mutate()} disabled={create.isPending}>
              <Plus className="h-4 w-4" />
              New session
            </Button>
          </div>
        </div>
        <div className="grid min-h-44 place-items-center rounded-lg bg-[var(--panel-strong)] p-5">
          <div className="grid w-full grid-cols-3 gap-3 text-center">
            <Metric icon={<Server />} label="API" value={health.data?.status ?? "checking"} />
            <Metric icon={<Database />} label="Redis" value={health.data?.redis_configured ? "ready" : "missing"} />
            <Metric icon={<Gauge />} label="Model" value={health.data?.model_configured ? "set" : "unset"} />
          </div>
        </div>
      </section>

      <section className="grid gap-5 lg:grid-cols-[1.15fr_0.85fr]">
        <div className="rounded-lg border border-[var(--line)] bg-white p-5">
          <div className="mb-4 flex items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold">Sessions</h2>
              <p className="text-sm text-[var(--muted)]">Switch from the IDE when you need another thread.</p>
            </div>
            <Badge>{sessions.data?.length ?? 0} total</Badge>
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
                  {session.active ? <Badge className="border-[var(--accent)] text-[var(--accent-strong)]">Active</Badge> : null}
                </div>
                <span className="truncate text-xs text-[var(--muted)]">{session.id}</span>
                <span className="text-xs text-[var(--muted)]">Updated {formatDateTime(session.updated_at)}</span>
              </Link>
            ))}
            {!sessions.data?.length ? (
              <p className="rounded-lg border border-dashed border-[var(--line)] p-4 text-sm text-[var(--muted)]">
                No sessions yet. Start one from the button above.
              </p>
            ) : null}
          </div>
        </div>

        <div className="grid gap-5">
          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <h2 className="text-lg font-semibold">Runtime</h2>
            <dl className="mt-4 grid gap-3 text-sm">
              <Row label="Provider" value={config.data?.provider ?? "unknown"} />
              <Row label="Model" value={config.data?.model ?? "unknown"} />
              <Row label="Agent" value={config.data?.agent?.mode ?? "react"} />
              <Row label="Plan" value={config.data?.agent?.plan_enabled ? "enabled" : "toggle in IDE"} />
              <Row label="Project" value={health.data?.project_root ?? config.data?.project_root ?? "unknown"} />
            </dl>
          </div>
          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="mb-3 flex items-center gap-2">
              <Sparkles className="h-4 w-4 text-[var(--accent-strong)]" />
              <h2 className="text-lg font-semibold">Skills</h2>
            </div>
            <p className="text-sm text-[var(--muted)]">
              {skills.data?.skills.length ?? 0} discovered,{" "}
              {skills.data?.skills.filter((skill) => skill.preloaded).length ?? 0} preloaded.
            </p>
          </div>
          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="mb-3 flex items-center gap-2">
              <PlugZap className="h-4 w-4 text-[var(--accent-strong)]" />
              <h2 className="text-lg font-semibold">MCP</h2>
            </div>
            <p className="text-sm text-[var(--muted)]">
              {mcp.data?.servers.length ?? 0} configured,{" "}
              {mcp.data?.servers.filter((server) => server.preloaded).length ?? 0} preloaded.
            </p>
          </div>
          <div className="rounded-lg border border-[var(--line)] bg-white p-5">
            <div className="mb-3 flex items-center gap-2">
              <Activity className="h-4 w-4 text-[var(--accent-strong)]" />
              <h2 className="text-lg font-semibold">Operation Timing</h2>
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
          <h2 className="text-lg font-semibold">Recent Audit</h2>
        </div>
        <div className="grid gap-2">
          {(audit.data ?? []).map((event, index) => (
            <div key={`${event.time}-${index}`} className="grid gap-1 rounded-lg border border-[var(--line)] p-3 md:grid-cols-[160px_1fr_120px]">
              <span className="text-xs text-[var(--muted)]">{formatDateTime(event.time)}</span>
              <span className="truncate text-sm">{event.args_summary || event.event}</span>
              <span className="text-xs text-[var(--muted)]">{event.confirmed === false ? "denied" : event.event}</span>
            </div>
          ))}
          {!audit.data?.length ? <p className="text-sm text-[var(--muted)]">No audit events yet.</p> : null}
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
