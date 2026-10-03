"use client";

import { motion } from "framer-motion";

// Inline SVG artwork for the home page. Colours come from theme aliases
// (stroke-panel-border, fill-panel, text-link, text-muted), so both themes work.

/** Faint grid behind the hero that fades out towards the edges. */
export function HeroGrid() {
  return (
    <svg
      aria-hidden="true"
      className="stroke-panel-border pointer-events-none absolute inset-0 h-full w-full [mask-image:radial-gradient(ellipse_at_center,black,transparent_75%)] opacity-60"
    >
      <defs>
        <pattern
          id="hero-grid"
          width="40"
          height="40"
          patternUnits="userSpaceOnUse"
        >
          <path d="M40 0H0V40" fill="none" strokeWidth="1" />
        </pattern>
      </defs>
      <rect width="100%" height="100%" fill="url(#hero-grid)" />
    </svg>
  );
}

interface Node {
  title: string;
  note: string;
  sandbox?: boolean;
}

const nodes: Node[] = [
  { title: "Browser", note: "Write and submit" },
  { title: "API", note: "Validate, save, queue" },
  { title: "Queue", note: "Redis Streams" },
  { title: "Runner", note: "Claims the job" },
  { title: "Sandbox", note: "nsjail + cgroup", sandbox: true },
];

const arrowDefs = (
  <defs>
    <marker
      id="pipe-arrow"
      viewBox="0 0 10 10"
      refX="8"
      refY="5"
      markerWidth="7"
      markerHeight="7"
      orient="auto-start-reverse"
    >
      <path
        d="M1 1 9 5 1 9"
        fill="none"
        className="stroke-link"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </marker>
  </defs>
);

function NodeBox({
  x,
  y,
  w,
  h,
  node,
  i,
}: {
  x: number;
  y: number;
  w: number;
  h: number;
  node: Node;
  i: number;
}) {
  return (
    <motion.g
      initial={{ opacity: 0, y: 8 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-40px" }}
      transition={{ duration: 0.4, delay: i * 0.12 }}
    >
      <rect
        x={x}
        y={y}
        width={w}
        height={h}
        rx="10"
        className={`fill-panel ${node.sandbox ? "stroke-link" : "stroke-panel-border"}`}
        strokeWidth={node.sandbox ? 1.75 : 1}
        strokeDasharray={node.sandbox ? "5 4" : undefined}
      />
      <text
        x={x + w / 2}
        y={y + h / 2 - 4}
        textAnchor="middle"
        className="fill-foreground text-[15px] font-semibold"
      >
        {node.title}
      </text>
      <text
        x={x + w / 2}
        y={y + h / 2 + 14}
        textAnchor="middle"
        className="fill-muted text-[12px]"
      >
        {node.note}
      </text>
    </motion.g>
  );
}

/** Submit-to-verdict pipeline: wide on desktop, stacked on phones. */
export function PipelineDiagram() {
  const w = 140;
  const h = 64;
  const gap = 55;
  return (
    <>
      <svg
        viewBox="0 0 960 200"
        role="img"
        aria-label="A submission goes from the browser to the API, into a queue, to a runner and into a sandbox; the verdict and live status come back to the browser."
        className="hidden h-auto w-full md:block"
      >
        {arrowDefs}
        {nodes.map((n, i) => (
          <NodeBox
            key={n.title}
            x={20 + i * (w + gap)}
            y={30}
            w={w}
            h={h}
            node={n}
            i={i}
          />
        ))}
        {nodes.slice(0, -1).map((n, i) => {
          const x1 = 20 + i * (w + gap) + w;
          return (
            <motion.line
              key={n.title}
              initial={{ pathLength: 0, opacity: 0 }}
              whileInView={{ pathLength: 1, opacity: 1 }}
              viewport={{ once: true, margin: "-40px" }}
              transition={{ duration: 0.4, delay: 0.15 + i * 0.12 }}

              x1={x1 + 4}
              y1={62}
              x2={x1 + gap - 4}
              y2={62}
              className="stroke-link"
              strokeWidth="1.6"
              markerEnd="url(#pipe-arrow)"
            />
          );
        })}
        <motion.path
          initial={{ pathLength: 0 }}
          whileInView={{ pathLength: 1 }}
          viewport={{ once: true, margin: "-40px" }}
          transition={{ duration: 0.9, delay: 0.8 }}
          d={`M ${20 + 3 * (w + gap) + w / 2} ${30 + h + 4} V 150 H ${20 + w / 2} V ${30 + h + 8}`}
          fill="none"
          className="stroke-link"
          strokeWidth="1.6"
          strokeDasharray="6 5"
          markerEnd="url(#pipe-arrow)"
        />
        <text
          x={(20 + w / 2 + 20 + 3 * (w + gap) + w / 2) / 2}
          y="176"
          textAnchor="middle"
          className="fill-muted text-[12px]"
        >
          Verdict and live status come back over SSE
        </text>
      </svg>

      <svg
        viewBox="0 0 330 490"
        role="img"
        aria-label="A submission goes from the browser to the API, into a queue, to a runner and into a sandbox; the verdict and live status come back to the browser."
        className="mx-auto h-auto w-full max-w-xs md:hidden"
      >
        {arrowDefs}
        {nodes.map((n, i) => (
          <NodeBox
            key={n.title}
            x={30}
            y={16 + i * 96}
            w={220}
            h={56}
            node={n}
            i={i}
          />
        ))}
        {nodes.slice(0, -1).map((n, i) => (
          <motion.line
            key={n.title}
            initial={{ pathLength: 0, opacity: 0 }}
            whileInView={{ pathLength: 1, opacity: 1 }}
            viewport={{ once: true, margin: "-40px" }}
            transition={{ duration: 0.4, delay: 0.15 + i * 0.12 }}
            x1={140}
            y1={16 + i * 96 + 60}
            x2={140}
            y2={16 + (i + 1) * 96 - 4}
            className="stroke-link"
            strokeWidth="1.6"
            markerEnd="url(#pipe-arrow)"
          />
        ))}
        <motion.path
          initial={{ pathLength: 0 }}
          whileInView={{ pathLength: 1 }}
          viewport={{ once: true, margin: "-40px" }}
          transition={{ duration: 0.9, delay: 0.8 }}
          d={`M 254 ${16 + 3 * 96 + 28} H 290 V ${16 + 28} H 254`}
          fill="none"
          className="stroke-link"
          strokeWidth="1.6"
          strokeDasharray="6 5"
          markerEnd="url(#pipe-arrow)"
        />
        <text
          x="312"
          y="180"
          textAnchor="middle"
          transform="rotate(-90 312 180)"
          className="fill-muted text-[11px]"
        >
          Verdict over SSE
        </text>
      </svg>
    </>
  );
}
