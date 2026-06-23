export function hasTrafficData(remote: number, cached: number, failed: number) {
  return remote + cached + failed > 0;
}
