import * as THREE from 'three';
import type { EntityView, Snapshot, Vec } from './api.generated';
import { makeModel, ownerColor } from './models';

type RenderEntity = { object: THREE.Group; animated: THREE.Object3D[]; from: THREE.Vector3; to: THREE.Vector3; at: number; signature: string; view: EntityView };
const defaultView = { zoom: 15, yaw: Math.PI / 4, tilt: Math.atan2(38, Math.hypot(30, 30)), distance: Math.hypot(30, 38, 30) };

export class WorldRenderer {
  readonly renderer: THREE.WebGLRenderer;
  readonly camera = new THREE.OrthographicCamera(-20, 20, 15, -15, .1, 300);
  readonly scene = new THREE.Scene();
  readonly target = new THREE.Vector3(19, 0, 42);
  readonly entities = new Map<number, RenderEntity>();
  readonly keys = new Set<string>();
  readonly selected = new Set<number>();
  readonly canvas: HTMLCanvasElement;
  private terrain?: THREE.InstancedMesh;
  private terrainKey = '';
  private snapshot?: Snapshot;
  private rings = new THREE.Group();
  private projectiles = new THREE.Group();
  private projectileMeshes = new Map<number, THREE.Mesh>();
  private projectileGeometry = { stone: new THREE.SphereGeometry(.12, 5, 4), arrow: new THREE.BoxGeometry(.035, .035, .4) };
  private projectileMaterial = { stone: new THREE.MeshBasicMaterial({ color: '#c2baa3' }), arrow: new THREE.MeshBasicMaterial({ color: '#604323' }) };
  private ringGeometry = new THREE.RingGeometry(1, 1.065, 48);
  private hovered: number | null = null;
  private raycaster = new THREE.Raycaster();
  private plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
  private ghost?: THREE.Group;
  private marker?: THREE.Mesh;
  private markerAt = 0;
  private zoom = defaultView.zoom;
  private yaw = defaultView.yaw;
  private tilt = defaultView.tilt;
  private lastFrame = performance.now();
  private resizeObserver: ResizeObserver;
  private animation = 0;
  private running = true;
  private reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  constructor(private container: HTMLElement) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false, powerPreference: 'high-performance' });
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 1.75));
    this.renderer.shadowMap.enabled = true; this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.setClearColor('#18231c'); this.canvas = this.renderer.domElement;
    this.canvas.setAttribute('aria-label', 'Interactive battlefield. Drag to rotate and tilt the view. Click to select, or Shift-drag to select a group. Use Pan view to move across the map. Choose Give order or press Q, then click to command.');
    this.canvas.tabIndex = 0; container.append(this.canvas);
    this.scene.add(new THREE.HemisphereLight('#fff4d4', '#73805f', 2.4));
    const sun = new THREE.DirectionalLight('#fff1d3', 3.1); sun.position.set(5, 60, 25); sun.castShadow = true;
    sun.shadow.mapSize.set(2048, 2048); sun.shadow.camera.left = -50; sun.shadow.camera.right = 50;
    sun.shadow.camera.top = 50; sun.shadow.camera.bottom = -50; sun.shadow.camera.far = 150; sun.shadow.bias = -.0007;
    sun.target.position.set(35, 0, 35); this.scene.add(sun, sun.target);
    this.scene.add(this.rings, this.projectiles);
    this.resizeObserver = new ResizeObserver(() => this.resize()); this.resizeObserver.observe(container); this.resize();
    this.canvas.addEventListener('wheel', e => {
      e.preventDefault();
      const unit = e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : e.deltaMode === WheelEvent.DOM_DELTA_PAGE ? this.container.clientHeight : 1;
      this.zoomBy(e.deltaY * unit * .012, { x: e.clientX, y: e.clientY });
    }, { passive: false });
    this.canvas.addEventListener('webglcontextlost', e => { e.preventDefault(); this.running = false; container.dispatchEvent(new CustomEvent('renderlost')); });
    this.canvas.addEventListener('webglcontextrestored', () => { this.running = true; this.terrainKey = ''; if (this.snapshot) this.update(this.snapshot); container.dispatchEvent(new CustomEvent('renderrestored')); });
    this.frame();
  }
  private resize() {
    const w = this.container.clientWidth, h = this.container.clientHeight;
    this.renderer.setSize(w, h); const aspect = w / Math.max(1, h);
    this.camera.left = -this.zoom * aspect; this.camera.right = this.zoom * aspect; this.camera.top = this.zoom; this.camera.bottom = -this.zoom;
    this.camera.updateProjectionMatrix(); this.positionCamera();
  }
  private positionCamera() {
    // At low angles the bottom of the orthographic view can otherwise start
    // below the ground. Retreat along the view direction without changing zoom.
    const distance = Math.max(defaultView.distance, (this.zoom * Math.cos(this.tilt) + 8) / Math.sin(this.tilt));
    const horizontal = distance * Math.cos(this.tilt);
    this.camera.position.copy(this.target).add(new THREE.Vector3(horizontal * Math.sin(this.yaw), distance * Math.sin(this.tilt), horizontal * Math.cos(this.yaw)));
    this.camera.lookAt(this.target); this.camera.updateMatrixWorld();
    this.container.dispatchEvent(new Event('viewchange'));
  }
  focus(position: Vec) { this.target.set(position.x, 0, position.y); this.positionCamera(); }
  zoomBy(delta: number, cursor?: { x: number; y: number }) {
    const zoom = THREE.MathUtils.clamp(this.zoom + delta, 7, 30);
    if (zoom === this.zoom) return;
    const before = cursor ? this.groundPoint(cursor.x, cursor.y) : null;
    this.zoom = zoom; this.resize();
    if (before && cursor) {
      const after = this.groundPoint(cursor.x, cursor.y);
      if (after) {
        // Offset the camera by the ground displacement caused by zooming, so
        // the same world point stays beneath the cursor at every view angle.
        this.target.x += before.x - after.x; this.target.z += before.y - after.y;
        this.positionCamera();
      }
    }
  }
  orbitBy(dx: number, dy: number) {
    this.yaw = (this.yaw - dx * .006) % (Math.PI * 2);
    this.tilt = THREE.MathUtils.clamp(this.tilt + dy * .004, Math.PI / 9, Math.PI * .44);
    this.positionCamera();
  }
  resetView() { this.yaw = defaultView.yaw; this.tilt = defaultView.tilt; this.zoom = defaultView.zoom; this.resize(); }
  pan(dx: number, dy: number) {
    // Convert screen motion to the ground plane at the current angle and zoom,
    // so dragging keeps the same ground point beneath the pointer.
    const scale = 2 * this.zoom / Math.max(1, this.container.clientHeight);
    const sideways = dx * scale, forward = dy * scale / Math.sin(this.tilt);
    this.target.x += Math.cos(this.yaw) * sideways + Math.sin(this.yaw) * forward;
    this.target.z += -Math.sin(this.yaw) * sideways + Math.cos(this.yaw) * forward;
    this.clampCamera();
  }
  private clampCamera() { this.target.x = THREE.MathUtils.clamp(this.target.x, 1, (this.snapshot?.map.width ?? 72) - 2); this.target.z = THREE.MathUtils.clamp(this.target.z, 1, (this.snapshot?.map.height ?? 72) - 2); this.positionCamera(); }
  private elevation(position: Vec) { const map = this.snapshot?.map; if (!map) return 0; return map.tiles[Math.floor(position.y) * map.width + Math.floor(position.x)]?.elevation ?? 0; }

  update(snapshot: Snapshot) {
    const previous = this.snapshot;
    this.snapshot = snapshot;
    const map = snapshot.map, key = map.fog.join('');
    if (this.terrain && this.terrain.count !== map.tiles.length) {
      this.scene.remove(this.terrain); this.terrain.geometry.dispose(); (this.terrain.material as THREE.Material).dispose(); this.terrain.dispose(); this.terrain = undefined;
    }
    if (!this.terrain || this.terrainKey !== key) {
      this.terrainKey = key;
      if (!this.terrain) {
        this.terrain = new THREE.InstancedMesh(new THREE.BoxGeometry(1.01, .3, 1.01), new THREE.MeshStandardMaterial({ roughness: 1 }), map.tiles.length);
        this.terrain.receiveShadow = true; this.terrain.frustumCulled = false; this.scene.add(this.terrain);
      }
      const matrix = new THREE.Matrix4(), color = new THREE.Color();
      map.tiles.forEach((tile, i) => {
        const x = i % map.width, y = Math.floor(i / map.width), fog = map.fog[i];
        const base = tile.terrain === 'water' ? '#668f8b' : tile.terrain === 'shallows' ? '#8fa393' : tile.terrain === 'cliff' ? '#929580' : '#9eae77';
        color.set(base);
        if (!fog) color.set('#1b2a22');
        else color.multiplyScalar((.96 + Math.sin(x * 14.3 + y * 7.8) * .055) * (fog === 1 ? .44 : 1));
        matrix.makeTranslation(x + .5, tile.elevation - .16, y + .5);
        this.terrain!.setMatrixAt(i, matrix); this.terrain!.setColorAt(i, color);
      });
      this.terrain.instanceMatrix.needsUpdate = true; if (this.terrain.instanceColor) this.terrain.instanceColor.needsUpdate = true;
    }
    const alive = new Set<number>(), now = performance.now();
    for (const e of snapshot.entities) {
      if (e.container) continue;
      alive.add(e.id);
      const signature = `${e.type}:${e.owner}:${e.visible}:${e.progress < 1}:${e.deployed}:${e.relic}`;
      let rendered = this.entities.get(e.id);
      if (!rendered || rendered.signature !== signature) {
        if (rendered) this.removeModel(rendered.object);
        const object = makeModel(e); this.scene.add(object);
        const pos = new THREE.Vector3(e.position.x, this.elevation(e.position), e.position.y);
        const animated: THREE.Object3D[] = [];
        object.traverse(o => { if (['leg', 'tool', 'windmill', 'flag'].includes(o.name)) animated.push(o); });
        rendered = { object, animated, from: pos.clone(), to: pos.clone(), at: now, signature, view: e }; this.entities.set(e.id, rendered);
      }
      rendered.from.copy(rendered.object.position); rendered.to.set(e.position.x, this.elevation(e.position), e.position.y);
      if (rendered.at === now) rendered.from.copy(rendered.to);
      rendered.at = now; rendered.view = e;
      if (e.kind === 'building') rendered.object.scale.y = .2 + .8 * e.progress;
    }
    for (const [id, e] of this.entities) if (!alive.has(id)) { this.removeModel(e.object); this.entities.delete(id); this.selected.delete(id); }
    this.updateRings();
    const projectiles = new Set(snapshot.projectiles.map(p => p.id));
    for (const [id, mesh] of this.projectileMeshes) if (!projectiles.has(id)) { this.projectiles.remove(mesh); this.projectileMeshes.delete(id); }
    const previousProjectiles = new Map(previous?.projectiles.map(p => [p.id, p]));
    for (const p of snapshot.projectiles) {
      let mesh = this.projectileMeshes.get(p.id);
      if (!mesh) { const kind = p.kind === 'stone' ? 'stone' : 'arrow'; mesh = new THREE.Mesh(this.projectileGeometry[kind], this.projectileMaterial[kind]); this.projectileMeshes.set(p.id, mesh); this.projectiles.add(mesh); }
      mesh.position.set(p.position.x, this.elevation(p.position) + 1.1, p.position.y);
      const last = previousProjectiles.get(p.id);
      if (last) mesh.lookAt(2 * p.position.x - last.position.x, mesh.position.y, 2 * p.position.y - last.position.y);
    }
  }
  private removeModel(object: THREE.Group) {
    this.scene.remove(object);
    object.traverse(o => { if (o instanceof THREE.Mesh && o.userData.privateMaterial) (o.material as THREE.Material).dispose(); if (o instanceof THREE.InstancedMesh) o.dispose(); });
  }
  private updateRings() {
    const wanted = new Set(this.selected); if (this.hovered !== null) wanted.add(this.hovered);
    for (const o of [...this.rings.children]) if (!wanted.has(o.userData.forEntity) || !this.entities.has(o.userData.forEntity)) {
      (o as THREE.Mesh<THREE.BufferGeometry, THREE.Material>).material.dispose(); this.rings.remove(o);
    }
    for (const id of wanted) {
      const e = this.entities.get(id); if (!e) continue;
      const radius = Math.max(.5, e.view.radius + .12);
      let ring = this.rings.children.find(r => r.userData.forEntity === id) as THREE.Mesh<THREE.BufferGeometry, THREE.MeshBasicMaterial> | undefined;
      if (!ring) { ring = new THREE.Mesh(this.ringGeometry, new THREE.MeshBasicMaterial({ side: THREE.DoubleSide, depthWrite: false })); ring.rotation.x = -Math.PI / 2; ring.userData.forEntity = id; this.rings.add(ring); }
      ring.scale.set(radius, radius, 1); ring.material.color.set(this.hovered === id && !this.selected.has(id) ? '#e3c984' : ownerColor(e.view.owner));
    }
  }
  setSelection(ids: number[]) { this.selected.clear(); ids.forEach(id => this.selected.add(id)); this.updateRings(); }
  setHovered(id: number | null) { if (this.hovered === id) return; this.hovered = id; this.updateRings(); }
  resetWorld() {
    this.preview(null); this.setHovered(null); this.selected.clear();
    for (const e of this.entities.values()) this.removeModel(e.object);
    this.entities.clear(); this.updateRings(); this.projectiles.clear(); this.projectileMeshes.clear();
    this.snapshot = undefined; this.terrainKey = ''; this.target.set(19, 0, 42); this.resetView();
  }
  groundPoint(clientX: number, clientY: number): Vec | null {
    const rect = this.canvas.getBoundingClientRect(); this.raycaster.setFromCamera(new THREE.Vector2((clientX - rect.left) / rect.width * 2 - 1, -(clientY - rect.top) / rect.height * 2 + 1), this.camera);
    const hit = this.raycaster.ray.intersectPlane(this.plane, new THREE.Vector3()); return hit ? { x: hit.x, y: hit.z } : null;
  }
  pick(clientX: number, clientY: number): number | null {
    this.groundPoint(clientX, clientY);
    const hits = this.raycaster.intersectObjects([...this.entities.values()].map(e => e.object), true);
    if (!hits.length) return null;
    let object: THREE.Object3D | null = hits[0].object;
    while (object && !object.userData.entityId) object = object.parent;
    return object?.userData.entityId ?? null;
  }
  screenPoint(id: number) {
    const e = this.entities.get(id); if (!e) return null;
    const p = e.object.position.clone().project(this.camera), r = this.canvas.getBoundingClientRect();
    return { x: (p.x + 1) / 2 * r.width + r.left, y: (-p.y + 1) / 2 * r.height + r.top };
  }
  preview(entity: EntityView | null, point?: Vec, valid?: boolean) {
    if (this.ghost) { this.ghost.traverse(o => { if (o instanceof THREE.Mesh) (o.material as THREE.Material).dispose(); if (o instanceof THREE.InstancedMesh) o.dispose(); }); this.scene.remove(this.ghost); this.ghost = undefined; }
    if (!entity || !point) return;
    this.ghost = makeModel(entity);
    this.ghost.position.set(Math.floor(point.x) + .5, this.elevation(point), Math.floor(point.y) + .5);
    this.ghost.traverse(o => { if (o instanceof THREE.Mesh) o.material = new THREE.MeshBasicMaterial({ color: valid === false ? '#d77f66' : valid === true ? '#c2d6a0' : '#d5c79e', transparent: true, opacity: .5 }); });
    this.scene.add(this.ghost);
  }
  orderMarker(point: Vec) {
    if (this.marker) { this.scene.remove(this.marker); this.marker.geometry.dispose(); (this.marker.material as THREE.Material).dispose(); }
    this.marker = new THREE.Mesh(new THREE.RingGeometry(.35, .43, 32), new THREE.MeshBasicMaterial({ color: '#fff2c0', transparent: true, side: THREE.DoubleSide }));
    this.marker.rotation.x = -Math.PI / 2; this.marker.position.set(point.x, this.elevation(point) + .08, point.y); this.markerAt = performance.now(); this.scene.add(this.marker);
  }
  private frame = () => {
    this.animation = requestAnimationFrame(this.frame);
    const now = performance.now(), dt = Math.min(.05, (now - this.lastFrame) / 1000); this.lastFrame = now;
    let dx = 0, dy = 0;
    if (this.keys.has('ArrowLeft')) dx -= 1; if (this.keys.has('ArrowRight')) dx += 1; if (this.keys.has('ArrowUp')) dy -= 1; if (this.keys.has('ArrowDown')) dy += 1;
    if (dx || dy) this.pan(dx * dt * 480, dy * dt * 480);
    for (const e of this.entities.values()) {
      const t = THREE.MathUtils.clamp((now - e.at) / 100, 0, 1);
      e.object.position.lerpVectors(e.from, e.to, t);
      const moving = e.from.distanceToSquared(e.to) > .0001;
      if (moving && e.view.kind === 'unit') e.object.rotation.y = Math.atan2(e.to.x - e.from.x, e.to.z - e.from.z) + Math.PI;
      e.animated.forEach(o => {
        if (this.reducedMotion.matches || this.snapshot?.paused) { if (o.name === 'leg' || o.name === 'tool') o.rotation.x = 0; return; }
        if (o.name === 'windmill') o.rotation.z = now * .00025;
        if (o.name === 'flag') o.rotation.y = Math.sin(now * .002 + e.view.id) * .1;
        if (o.name === 'leg') o.rotation.x = moving ? Math.sin(now * .012 + o.position.x * 10) * .35 : 0;
        if (o.name === 'tool') o.rotation.x = ['gathering', 'constructing', 'repairing', 'attacking'].includes(e.view.state) ? Math.sin(now * .008) * .4 : 0;
      });
    }
    this.rings.children.forEach(r => { const e = this.entities.get(r.userData.forEntity); if (e) { r.position.copy(e.object.position); r.position.y += .04; } });
    if (this.marker) { const age = (now - this.markerAt) / 800; this.marker.scale.setScalar(1 + age); (this.marker.material as THREE.MeshBasicMaterial).opacity = Math.max(0, 1 - age); }
    if (this.running) this.renderer.render(this.scene, this.camera);
  };
  dispose() { cancelAnimationFrame(this.animation); this.resizeObserver.disconnect(); this.renderer.dispose(); }
}
