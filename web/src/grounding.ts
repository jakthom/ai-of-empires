import * as THREE from 'three';
import type { EntityView, Vec } from './api.generated';

export type TerrainSurface = { readonly revision: number; height(position: Vec): number };

// These meshes project Go's terrain into the scene. They never change an
// entity's horizontal position, collision footprint, or gameplay elevation.
export class GroundRing extends THREE.Mesh<THREE.RingGeometry, THREE.MeshBasicMaterial> {
  private readonly flat: Float32Array;
  private lastX = NaN;
  private lastZ = NaN;
  private lastRadius = NaN;
  private lastTerrain?: TerrainSurface;
  private lastRevision = -1;

  constructor(outer = 1.065, fading = false) {
    super(new THREE.RingGeometry(1, outer, 64).rotateX(-Math.PI / 2), new THREE.MeshBasicMaterial({
      side: THREE.DoubleSide, depthWrite: false, transparent: fading,
      polygonOffset: true, polygonOffsetFactor: -1, polygonOffsetUnits: -1,
    }));
    this.flat = new Float32Array(this.geometry.getAttribute('position').array);
    this.renderOrder = 1;
  }

  place(center: Vec, radius: number, terrain?: TerrainSurface) {
    if (center.x === this.lastX && center.y === this.lastZ && radius === this.lastRadius && terrain === this.lastTerrain && (terrain?.revision ?? -1) === this.lastRevision) return;
    this.lastX = center.x; this.lastZ = center.y; this.lastRadius = radius;
    this.lastTerrain = terrain; this.lastRevision = terrain?.revision ?? -1;
    const positions = this.geometry.getAttribute('position');
    for (let i = 0; i < positions.count; i++) {
      const x = this.flat[i * 3] * radius, z = this.flat[i * 3 + 2] * radius;
      positions.setXYZ(i, x, (terrain?.height({ x: center.x + x, y: center.y + z }) ?? 0) + .018, z);
    }
    positions.needsUpdate = true; this.geometry.computeBoundingSphere();
    this.position.set(center.x, 0, center.y);
  }

  dispose() { this.geometry.dispose(); this.material.dispose(); }
}

export class BuildingFoundation extends THREE.Mesh<THREE.BufferGeometry, THREE.MeshStandardMaterial> {
  private lastTerrain?: TerrainSurface;
  private lastRevision = -1;
  private lastX = NaN;
  private lastZ = NaN;
  private lastScale = NaN;
  private readonly halfWidth: number;
  private readonly halfDepth: number;
  private readonly top: number;
  elevation = 0;

  constructor(entity: EntityView) {
    super(new THREE.BufferGeometry(), new THREE.MeshStandardMaterial({ color: entity.type === 'palisade' ? '#6e4f36' : '#817962', roughness: 1 }));
    const wall = ['wall', 'gate', 'palisade'].includes(entity.type);
    this.halfWidth = entity.radius * (wall ? .9 : 1); this.halfDepth = entity.radius * (wall ? .65 : 1);
    this.top = wall ? 0 : -.04;
    this.name = 'foundation'; this.castShadow = true; this.receiveShadow = true;
    this.userData.privateGeometry = true; this.userData.privateMaterial = true;
  }

  fit(center: Vec, terrain: TerrainSurface, scale = 1) {
    if (center.x === this.lastX && center.y === this.lastZ && scale === this.lastScale && terrain === this.lastTerrain && terrain.revision === this.lastRevision) return this.elevation;
    this.lastX = center.x; this.lastZ = center.y; this.lastScale = scale; this.lastTerrain = terrain; this.lastRevision = terrain.revision;
    // A level floor clears the highest ground anywhere inside its footprint.
    let highest = -Infinity;
    const nx = Math.ceil(this.halfWidth * 8), nz = Math.ceil(this.halfDepth * 8);
    for (let iz = 0; iz <= nz; iz++) for (let ix = 0; ix <= nx; ix++) {
      highest = Math.max(highest, terrain.height({ x: center.x - this.halfWidth + ix / nx * this.halfWidth * 2, y: center.y - this.halfDepth + iz / nz * this.halfDepth * 2 }));
    }
    const top = this.top * scale;
    this.elevation = highest - top + .01; this.scale.y = 1 / scale;
    const positions: number[] = [];
    const corners = [[-this.halfWidth, -this.halfDepth], [-this.halfWidth, this.halfDepth], [this.halfWidth, this.halfDepth], [this.halfWidth, -this.halfDepth]];
    for (let side = 0; side < 4; side++) {
      const a = corners[side], b = corners[(side + 1) % 4], steps = Math.ceil(Math.hypot(b[0] - a[0], b[1] - a[1]) * 4);
      for (let i = 0; i < steps; i++) {
        const ax = a[0] + (b[0] - a[0]) * i / steps, az = a[1] + (b[1] - a[1]) * i / steps;
        const bx = a[0] + (b[0] - a[0]) * (i + 1) / steps, bz = a[1] + (b[1] - a[1]) * (i + 1) / steps;
        const ay = terrain.height({ x: center.x + ax, y: center.y + az }) - this.elevation - .03;
        const by = terrain.height({ x: center.x + bx, y: center.y + bz }) - this.elevation - .03;
        positions.push(ax, top, az, ax, ay, az, bx, by, bz, ax, top, az, bx, by, bz, bx, top, bz);
      }
    }
    this.geometry.dispose(); this.geometry = new THREE.BufferGeometry();
    this.geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3));
    this.geometry.computeVertexNormals(); this.geometry.computeBoundingSphere();
    return this.elevation;
  }
}

export function groundFarm(model: THREE.Group, center: Vec, terrain: TerrainSurface) {
  const base = terrain.height(center), matrix = new THREE.Matrix4();
  model.traverse(object => {
    if (!(object instanceof THREE.Mesh)) return;
    if (object instanceof THREE.InstancedMesh) {
      for (let i = 0; i < object.count; i++) {
        object.getMatrixAt(i, matrix);
        matrix.elements[13] += terrain.height({ x: center.x + matrix.elements[12], y: center.y + matrix.elements[14] }) - base;
        object.setMatrixAt(i, matrix);
      }
      object.instanceMatrix.needsUpdate = true; object.computeBoundingSphere();
    } else {
      if (!object.userData.privateGeometry) { object.geometry = object.geometry.clone(); object.userData.privateGeometry = true; }
      object.updateMatrix(); object.geometry.applyMatrix4(object.matrix);
      object.position.set(0, 0, 0); object.quaternion.identity(); object.scale.set(1, 1, 1);
      const positions = object.geometry.getAttribute('position');
      for (let i = 0; i < positions.count; i++) positions.setY(i, positions.getY(i) + terrain.height({ x: center.x + positions.getX(i), y: center.y + positions.getZ(i) }) - base);
      positions.needsUpdate = true; object.geometry.computeVertexNormals(); object.geometry.computeBoundingBox(); object.geometry.computeBoundingSphere();
    }
  });
}
