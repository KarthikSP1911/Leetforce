import Link from "next/link";

export function Pagination({
  page,
  pageCount,
  hrefFor,
}: {
  page: number;
  pageCount: number;
  hrefFor: (page: number) => string;
}) {
  if (pageCount <= 1) return null;
  const linkClass =
    "bg-panel border-panel-border hover:bg-hover rounded-lg border px-3 py-1.5 text-sm";
  const off = `${linkClass} text-muted pointer-events-none opacity-50`;
  return (
    <nav
      aria-label="Pagination"
      className="mt-4 flex items-center justify-between"
    >
      <span className="text-muted text-sm">
        Page {page} of {pageCount}
      </span>
      <div className="flex gap-2">
        {page > 1 ? (
          <Link href={hrefFor(page - 1)} className={linkClass} rel="prev">
            Previous
          </Link>
        ) : (
          <span aria-disabled="true" className={off}>
            Previous
          </span>
        )}
        {page < pageCount ? (
          <Link href={hrefFor(page + 1)} className={linkClass} rel="next">
            Next
          </Link>
        ) : (
          <span aria-disabled="true" className={off}>
            Next
          </span>
        )}
      </div>
    </nav>
  );
}
