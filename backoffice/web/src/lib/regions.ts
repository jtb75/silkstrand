/**
 * Map a GCP region code to a high-level world region for UI grouping.
 * Ref: https://cloud.google.com/about/locations
 */
export type WorldRegion = 'Americas' | 'EMEA' | 'APAC';

export const WORLD_REGIONS: WorldRegion[] = ['Americas', 'EMEA', 'APAC'];

export function worldRegionForGCP(region: string): WorldRegion {
  const r = region.toLowerCase();
  if (
    r.startsWith('us-') ||
    r.startsWith('northamerica-') ||
    r.startsWith('southamerica-')
  ) {
    return 'Americas';
  }
  if (r.startsWith('europe-') || r.startsWith('me-') || r.startsWith('africa-')) {
    return 'EMEA';
  }
  // asia-*, australia-*
  return 'APAC';
}
