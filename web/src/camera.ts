import { Plane, Ray, Vector3 } from 'three';

// Translate a view horizontally until its cursor ray passes through the
// grabbed terrain point. The camera pivot's height must never accumulate
// changes when the player alternates between zooming over hills and valleys.
export function terrainAnchorOffset(ray: Ray, anchor: Vector3): Vector3 | null {
  const at = ray.intersectPlane(new Plane(new Vector3(0, 1, 0), -anchor.y), new Vector3());
  return at ? new Vector3(anchor.x - at.x, 0, anchor.z - at.z) : null;
}
