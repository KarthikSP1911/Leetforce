import type { ContestStatus } from "@/types/contest";

const style: Record<ContestStatus, string> = {
  upcoming: "text-muted border-panel-border",
  running: "text-success border-success",
  ended: "text-muted border-panel-border",
};

const label: Record<ContestStatus, string> = {
  upcoming: "Upcoming",
  running: "Running",
  ended: "Ended",
};

export function StatusBadge({ status }: { status: ContestStatus }) {
  return (
    <span
      className={`inline-block rounded-full border px-2 py-0.5 text-xs font-semibold ${style[status]}`}
    >
      {label[status]}
    </span>
  );
}

export function formatTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}
