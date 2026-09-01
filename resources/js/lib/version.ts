export function formatVersion(version: string): string {
  const normalized = version.replace(/^v/, "").trim();
  if (!normalized) return "Unavailable";
  if (normalized === "dev") return "Development build";
  if (
    normalized.startsWith("edge-") ||
    normalized.startsWith("development-")
  ) {
    return normalized;
  }
  return `v${normalized}`;
}

export function releaseUpdateSummary(version: string): string {
  const formatted = formatVersion(version);
  if (formatted === "Unavailable") return "Update DeployCrate CE";
  return `Update DeployCrate CE to ${formatted}`;
}
