type Point = { x: number; y: number; z: number };
type Observation = Point & { time: number };

// Presentation has a short, adaptive delay behind observed server wall time.
// It never multiplies by game speed or extrapolates an unobserved position.
export class ObservationClock {
  private latest = 0;
  private offset = 0;
  private interval = 50;
  private jitter = 0;
  private playhead = 0;
  private epoch?: object;

  observe(sample: number, received: number, epoch: object): boolean {
    const reset = this.epoch !== epoch || sample < this.latest || sample - this.latest > 2000;
    if (reset) {
      this.epoch = epoch; this.latest = sample; this.offset = received - sample;
      this.interval = 50; this.jitter = 0; this.playhead = sample;
      return true;
    }
    if (sample > this.latest) {
      const gap = sample - this.latest;
      const delivery = received - sample - this.offset;
      this.jitter = Math.max(this.jitter * .95, Math.min(150, Math.abs(delivery)));
      this.interval += (Math.min(150, gap) - this.interval) * .1;
      this.offset = Math.min(this.offset, received - sample);
      this.latest = sample;
    }
    return false;
  }

  time(now: number): number {
    const delay = Math.max(100, Math.min(300, this.interval * 2 + this.jitter));
    this.playhead = Math.max(this.playhead, Math.min(this.latest, now - this.offset - delay));
    return this.playhead;
  }
}

export class ObservedMotion {
  private samples: Observation[] = [];

  add(time: number, point: Point, reset = false) {
    if (reset) this.samples = [];
    const last = this.samples.at(-1);
    if (last?.time === time) this.samples.pop();
    else if (last && last.time > time) return;
    this.samples.push({ time, x: point.x, y: point.y, z: point.z });
    // Eight 20 Hz observations cover the bounded delay without an ever-growing
    // movement history. Reconnects and hidden/removed entities clear the buffer.
    if (this.samples.length > 8) this.samples.shift();
  }

  sample(time: number, out: Point): boolean {
    if (!this.samples.length) return false;
    while (this.samples.length > 2 && this.samples[1].time <= time) this.samples.shift();
    const a = this.samples[0], b = this.samples[1] ?? a;
    const fraction = b.time > a.time ? Math.max(0, Math.min(1, (time - a.time) / (b.time - a.time))) : 1;
    out.x = a.x + (b.x - a.x) * fraction;
    out.y = a.y + (b.y - a.y) * fraction;
    out.z = a.z + (b.z - a.z) * fraction;
    return time < b.time && time >= a.time && (a.x !== b.x || a.y !== b.y || a.z !== b.z);
  }
}
