import { useEffect, useRef } from "react";

// 氛围层：低分辨率 canvas 上缓慢漂移的暖光斑（夜市灯光）+ 纸面颗粒叠加。
// prefers-reduced-motion 时只画一帧静态光；颗粒是纯静态纹理。
const BLOBS = [
  {
    rgb: "255, 214, 156",
    alpha: 0.4,
    radius: 360,
    cx: 0.2,
    cy: 0.05,
    dx: 0.06,
    dy: 0.05,
    speed: 0.00005,
    phase: 0,
  },
  {
    rgb: "216, 92, 39",
    alpha: 0.1,
    radius: 300,
    cx: 0.86,
    cy: 0.3,
    dx: 0.05,
    dy: 0.08,
    speed: 0.00004,
    phase: 2.1,
  },
  {
    rgb: "47, 125, 140",
    alpha: 0.07,
    radius: 280,
    cx: 0.68,
    cy: 0.95,
    dx: 0.08,
    dy: 0.04,
    speed: 0.00003,
    phase: 4.4,
  },
] as const;

export function AmbientBackdrop() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext("2d");
    if (!canvas || !context) {
      return;
    }

    // 半分辨率渲染，CSS 拉伸自带柔化
    const resize = () => {
      canvas.width = Math.ceil(window.innerWidth / 2);
      canvas.height = Math.ceil(window.innerHeight / 2);
    };
    resize();

    const draw = (time: number) => {
      const { width, height } = canvas;
      context.clearRect(0, 0, width, height);
      const scale = width / 720 + 0.4;
      for (const blob of BLOBS) {
        const x =
          (blob.cx + Math.sin(time * blob.speed + blob.phase) * blob.dx) *
          width;
        const y =
          (blob.cy + Math.cos(time * blob.speed * 0.8 + blob.phase) * blob.dy) *
          height;
        const radius = blob.radius * scale;
        const gradient = context.createRadialGradient(x, y, 0, x, y, radius);
        gradient.addColorStop(0, `rgba(${blob.rgb}, ${blob.alpha})`);
        gradient.addColorStop(1, `rgba(${blob.rgb}, 0)`);
        context.fillStyle = gradient;
        context.fillRect(0, 0, width, height);
      }
    };

    let frame = 0;
    const loop = (time: number) => {
      draw(time);
      frame = requestAnimationFrame(loop);
    };
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      draw(0);
    } else {
      frame = requestAnimationFrame(loop);
    }
    window.addEventListener("resize", resize);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", resize);
    };
  }, []);

  return (
    <div aria-hidden="true">
      <div className="ambient">
        <canvas ref={canvasRef} className="ambient-canvas" />
      </div>
      <div className="grain-overlay" />
    </div>
  );
}
