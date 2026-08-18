import { defineMiddleware } from "astro:middleware";
import { readFile } from "node:fs/promises";
import { extname, resolve, sep } from "node:path";
import { pages } from "./lib/catalog";

const directoryRoutes = new Map(
  pages
    .filter((page) => page.slug.endsWith("/index"))
    .map((page) => [`/${page.slug.slice(0, -"/index".length)}`, `/${page.slug}`]),
);
const reportPrefix = "/reports/tests/latest/";
const reportContentTypes: Record<string, string> = {
  ".json": "application/json; charset=utf-8",
  ".log": "text/plain; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".xml": "application/xml; charset=utf-8",
};

async function generatedReportResponse(pathname: string): Promise<Response | undefined> {
  const workspace = process.env.BUILD_WORKSPACE_DIRECTORY;
  if (!workspace || !pathname.startsWith(reportPrefix)) return undefined;

  const relative = pathname.slice(reportPrefix.length);
  if (!relative || relative.split("/").some((part) => part === "..")) return undefined;

  const reportRoot = resolve(workspace, ".test-report/evidence");
  const reportPath = resolve(reportRoot, relative);
  if (reportPath !== reportRoot && !reportPath.startsWith(`${reportRoot}${sep}`)) return undefined;

  try {
    const body = await readFile(reportPath);
    return new Response(body, {
      headers: {
        "Cache-Control": "no-store",
        "Content-Type": reportContentTypes[extname(reportPath)] ?? "application/octet-stream",
      },
    });
  } catch {
    return undefined;
  }
}

export const onRequest = defineMiddleware(async (context, next) => {
  const report = await generatedReportResponse(context.url.pathname);
  if (report) return report;

  const pathname = context.url.pathname.replace(/\/$/, "");
  const indexRoute = directoryRoutes.get(pathname);
  if (indexRoute) return context.rewrite(indexRoute);
  return next();
});
