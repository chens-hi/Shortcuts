const fs = require('fs');
const path = require('path');
const zlib = require('zlib');

const OUT_DIR = __dirname;

function clamp(value, min = 0, max = 255) {
  return Math.max(min, Math.min(max, value));
}

function hex(color) {
  const value = parseInt(color.slice(1), 16);
  return {
    r: (value >> 16) & 255,
    g: (value >> 8) & 255,
    b: value & 255,
    a: 255,
  };
}

function rgba(color, alpha = 1) {
  const c = typeof color === 'string' ? hex(color) : color;
  return [clamp(c.r), clamp(c.g), clamp(c.b), clamp(Math.round(alpha * 255))];
}

function lerp(a, b, t) {
  return a + (b - a) * t;
}

function mix(c1, c2, t) {
  return {
    r: lerp(c1.r, c2.r, t),
    g: lerp(c1.g, c2.g, t),
    b: lerp(c1.b, c2.b, t),
    a: lerp(c1.a, c2.a, t),
  };
}

function insideRoundedRect(px, py, rect, radius) {
  const { x, y, w, h } = rect;
  if (px < x || px > x + w || py < y || py > y + h) return false;
  const cx = Math.max(x + radius, Math.min(px, x + w - radius));
  const cy = Math.max(y + radius, Math.min(py, y + h - radius));
  return (px - cx) ** 2 + (py - cy) ** 2 <= radius * radius;
}

function roundedRectMask(px, py, rect, radius) {
  if (px < rect.x || px > rect.x + rect.w || py < rect.y || py > rect.y + rect.h) {
    return 0;
  }
  const cx = Math.max(rect.x + radius, Math.min(px, rect.x + rect.w - radius));
  const cy = Math.max(rect.y + radius, Math.min(py, rect.y + rect.h - radius));
  const d = Math.hypot(px - cx, py - cy) - radius;
  if (d <= -0.75) return 1;
  if (d >= 0.75) return 0;
  return (0.75 - d) / 1.5;
}

function polygonMask(px, py, points) {
  let inside = false;
  for (let i = 0, j = points.length - 1; i < points.length; j = i++) {
    const xi = points[i][0];
    const yi = points[i][1];
    const xj = points[j][0];
    const yj = points[j][1];
    if ((yi > py) !== (yj > py) && px < ((xj - xi) * (py - yi)) / (yj - yi) + xi) {
      inside = !inside;
    }
  }
  return inside ? 1 : 0;
}

function circleMask(px, py, center, radius) {
  const d = Math.hypot(px - center[0], py - center[1]) - radius;
  if (d <= -0.75) return 1;
  if (d >= 0.75) return 0;
  return (0.75 - d) / 1.5;
}

function diamondPoints(cx, cy, radius) {
  return [
    [cx, cy - radius],
    [cx + radius, cy],
    [cx, cy + radius],
    [cx - radius, cy],
  ];
}

function blendAt(buffer, offset, src, alpha) {
  const a = alpha * (src[3] / 255);
  if (a <= 0) return;
  const inv = 1 - a;
  buffer[offset] = src[0] * a + buffer[offset] * inv;
  buffer[offset + 1] = src[1] * a + buffer[offset + 1] * inv;
  buffer[offset + 2] = src[2] * a + buffer[offset + 2] * inv;
  buffer[offset + 3] = src[3] * alpha + buffer[offset + 3] * inv;
}

function drawRoundedRect(size, buffer, rect, radius, fill, alpha = 1) {
  const minX = Math.max(0, Math.floor(rect.x));
  const maxX = Math.min(size - 1, Math.ceil(rect.x + rect.w));
  const minY = Math.max(0, Math.floor(rect.y));
  const maxY = Math.min(size - 1, Math.ceil(rect.y + rect.h));
  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      const a = roundedRectMask(x + 0.5, y + 0.5, rect, radius) * alpha;
      if (a > 0) blendAt(buffer, (y * size + x) * 4, fill, a);
    }
  }
}

function drawGradientRoundedRect(size, buffer, rect, radius, stops, alpha = 1) {
  const minX = Math.max(0, Math.floor(rect.x));
  const maxX = Math.min(size - 1, Math.ceil(rect.x + rect.w));
  const minY = Math.max(0, Math.floor(rect.y));
  const maxY = Math.min(size - 1, Math.ceil(rect.y + rect.h));
  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      const a = roundedRectMask(x + 0.5, y + 0.5, rect, radius) * alpha;
      if (a <= 0) continue;
      const t = ((x - rect.x) / rect.w + (y - rect.y) / rect.h) / 2;
      let color = stops[stops.length - 1];
      for (let i = 0; i < stops.length - 1; i++) {
        const from = stops[i];
        const to = stops[i + 1];
        if (t >= from.pos && t <= to.pos) {
          color = mix(from.color, to.color, (t - from.pos) / (to.pos - from.pos || 1));
          break;
        }
      }
      blendAt(buffer, (y * size + x) * 4, rgba(color, alpha), a);
    }
  }
}

function drawPolygon(size, buffer, points, fill, alpha = 1) {
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  for (const [x, y] of points) {
    minX = Math.min(minX, x);
    maxX = Math.max(maxX, x);
    minY = Math.min(minY, y);
    maxY = Math.max(maxY, y);
  }
  for (let y = Math.max(0, Math.floor(minY)); y <= Math.min(size - 1, Math.ceil(maxY)); y++) {
    for (let x = Math.max(0, Math.floor(minX)); x <= Math.min(size - 1, Math.ceil(maxX)); x++) {
      const a = polygonMask(x + 0.5, y + 0.5, points) * alpha;
      if (a > 0) blendAt(buffer, (y * size + x) * 4, fill, a);
    }
  }
}

function drawCircle(size, buffer, center, radius, fill, alpha = 1) {
  for (let y = Math.max(0, Math.floor(center[1] - radius)); y <= Math.min(size - 1, Math.ceil(center[1] + radius)); y++) {
    for (let x = Math.max(0, Math.floor(center[0] - radius)); x <= Math.min(size - 1, Math.ceil(center[0] + radius)); x++) {
      const a = circleMask(x + 0.5, y + 0.5, center, radius) * alpha;
      if (a > 0) blendAt(buffer, (y * size + x) * 4, fill, a);
    }
  }
}

function drawRing(size, buffer, center, innerR, outerR, fill, alpha = 1) {
  for (let y = Math.max(0, Math.floor(center[1] - outerR)); y <= Math.min(size - 1, Math.ceil(center[1] + outerR)); y++) {
    for (let x = Math.max(0, Math.floor(center[0] - outerR)); x <= Math.min(size - 1, Math.ceil(center[0] + outerR)); x++) {
      const d = Math.sqrt((x + 0.5 - center[0]) ** 2 + (y + 0.5 - center[1]) ** 2);
      if (d > outerR || d < innerR) continue;
      const outerAlpha = d > outerR - 1.5 ? (outerR - d) / 1.5 : 1;
      const innerAlpha = d < innerR + 1.5 ? (d - innerR) / 1.5 : 1;
      const a = Math.min(outerAlpha, innerAlpha) * alpha;
      if (a > 0) blendAt(buffer, (y * size + x) * 4, fill, a);
    }
  }
}

function drawSoftCircle(size, buffer, center, radius, color, alpha = 1, softness = 0.3) {
  for (let y = Math.max(0, Math.floor(center[1] - radius)); y <= Math.min(size - 1, Math.ceil(center[1] + radius)); y++) {
    for (let x = Math.max(0, Math.floor(center[0] - radius)); x <= Math.min(size - 1, Math.ceil(center[0] + radius)); x++) {
      const d = Math.hypot(x + 0.5 - center[0], y + 0.5 - center[1]);
      if (d > radius) continue;
      const t = (radius - d) / (radius * softness);
      const a = Math.min(1, Math.max(0, t)) * alpha;
      if (a > 0) blendAt(buffer, (y * size + x) * 4, rgba(color, 1), a);
    }
  }
}

function drawGradientArc(size, buffer, center, radius, startAngle, endAngle, stops, width, alpha = 1) {
  const innerR = radius - width / 2;
  const outerR = radius + width / 2;
  const span = endAngle - startAngle;
  
  for (let y = Math.max(0, Math.floor(center[1] - outerR)); y <= Math.min(size - 1, Math.ceil(center[1] + outerR)); y++) {
    for (let x = Math.max(0, Math.floor(center[0] - outerR)); x <= Math.min(size - 1, Math.ceil(center[0] + outerR)); x++) {
      const dx = x + 0.5 - center[0];
      const dy = y + 0.5 - center[1];
      const d = Math.sqrt(dx * dx + dy * dy);
      if (d > outerR || d < innerR) continue;
      
      // 计算当前点的角度
      let angle = Math.atan2(dy, dx);
      // 归一化到 [0, 2π)
      if (angle < 0) angle += 2 * Math.PI;
      
      // 计算相对起始角度的位置
      let relAngle = angle - startAngle;
      while (relAngle < 0) relAngle += 2 * Math.PI;
      while (relAngle >= 2 * Math.PI) relAngle -= 2 * Math.PI;
      
      // 检查是否在弧范围内
      if (relAngle > span) continue;
      
      // 计算渐变位置 t
      const t = relAngle / span;
      let color = stops[stops.length - 1].color;
      for (let i = 0; i < stops.length - 1; i++) {
        const from = stops[i];
        const to = stops[i + 1];
        if (t >= from.pos && t <= to.pos) {
          color = mix(from.color, to.color, (t - from.pos) / (to.pos - from.pos || 1));
          break;
        }
      }
      
      // 边缘抗锯齿
      const outerAlpha = d > outerR - 1.5 ? (outerR - d) / 1.5 : 1;
      const innerAlpha = d < innerR + 1.5 ? (d - innerR) / 1.5 : 1;
      const a = Math.min(outerAlpha, innerAlpha) * alpha;
      
      if (a > 0) blendAt(buffer, (y * size + x) * 4, rgba(color, a), a);
    }
  }
}

function drawGradientCircle(size, buffer, center, radius, stops, alpha = 1) {
  for (let y = Math.max(0, Math.floor(center[1] - radius)); y <= Math.min(size - 1, Math.ceil(center[1] + radius)); y++) {
    for (let x = Math.max(0, Math.floor(center[0] - radius)); x <= Math.min(size - 1, Math.ceil(center[0] + radius)); x++) {
      const a = circleMask(x + 0.5, y + 0.5, center, radius) * alpha;
      if (a <= 0) continue;
      const gradientT = Math.max(0, Math.min(1, ((y + 0.5 - (center[1] - radius)) / (radius * 2))));
      const t = Math.round(gradientT * 1000) / 1000;
      let color = stops[stops.length - 1].color;
      for (let i = 0; i < stops.length - 1; i++) {
        const from = stops[i];
        const to = stops[i + 1];
        if (t >= from.pos && t <= to.pos) {
          color = mix(from.color, to.color, (t - from.pos) / (to.pos - from.pos || 1));
          break;
        }
      }
      blendAt(buffer, (y * size + x) * 4, rgba(color, alpha), a);
    }
  }
}

function createImage(size) {
  return { size, data: Buffer.alloc(size * size * 4) };
}

function drawMainIcon(size, phase = 0) {
  const image = createImage(size);
  const buffer = image.data;
  const s = size / 1024;
  const safe = { x: 0, y: 0, w: 1024 * s, h: 1024 * s };
  const radius = 224 * s;

  // Full-bleed rounded rect matching the iOS-style squircle corner
  // (radius ≈ 22% of canvas), as referenced by Moviepilot_A.png.
  drawGradientRoundedRect(size, buffer, safe, radius, [
    { pos: 0, color: hex('#72abff') },
    { pos: 0.48, color: hex('#4d7bfd') },
    { pos: 1, color: hex('#2f4ff3') },
  ]);

  // APNG 动画：上菱形组与下菱形组做对向"呼吸"。
  // θ=0（phase=0）时 sin=0，几何与静态图完全一致，保证降级场景不变。
  const theta = 2 * Math.PI * phase;
  const bob = 24 * s * Math.sin(theta);
  const topScale = 1 + 0.04 * Math.sin(theta);
  const bottomScale = 1 - 0.04 * Math.sin(theta);

  const bottomRadius = 268 * s;
  const bottomCenter = [512 * s, 614 * s + bob];
  drawPolygon(size, buffer, diamondPoints(bottomCenter[0], bottomCenter[1], bottomRadius * bottomScale), rgba('#17bcd9', 1));
  drawPolygon(size, buffer, diamondPoints(bottomCenter[0], bottomCenter[1], 196 * s * bottomScale), rgba('#8bf2ff', 1));

  const topCenter = [512 * s, 388 * s - bob];
  drawPolygon(size, buffer, diamondPoints(topCenter[0], topCenter[1], 248 * s * topScale), rgba('#8c4ef6', 1));
  drawPolygon(size, buffer, diamondPoints(topCenter[0], topCenter[1], 178 * s * topScale), rgba('#c8a0ff', 1));
  drawCircle(size, buffer, topCenter, 60 * s * topScale, rgba('#ffffff', 0.98));
  drawCircle(size, buffer, topCenter, 31 * s * topScale, rgba('#ffd15f', 0.98));

  return image;
}

function drawDefaultEntryIcon(size, phase = 0) {
  const image = createImage(size);
  const buffer = image.data;
  const s = size / 1024;
  const safe = { x: 0, y: 0, w: 1024 * s, h: 1024 * s };
  const radius = 224 * s;

  drawGradientRoundedRect(size, buffer, safe, radius, [
    { pos: 0, color: hex('#72abff') },
    { pos: 0.48, color: hex('#4d7bfd') },
    { pos: 1, color: hex('#2f4ff3') },
  ]);

  // 双环并排 + 呼吸
  const theta = 2 * Math.PI * phase;
  const ringW = 28 * s;
  const ringR = 180 * s;
  const leftCenter = [346 * s, 512 * s];
  const rightCenter = [678 * s, 512 * s];

  // 左环呼吸（紫色）
  const leftPulse = 1 + 0.08 * Math.sin(theta);
  drawRing(size, buffer, leftCenter, (ringR - ringW) * leftPulse, ringR * leftPulse, rgba('#c8a0ff', 0.7));

  // 右环呼吸（青色）反相
  const rightPulse = 1 + 0.08 * Math.sin(theta + Math.PI);
  drawRing(size, buffer, rightCenter, (ringR - ringW) * rightPulse, ringR * rightPulse, rgba('#8bf2ff', 0.7));

  return image;
}

function drawComparison(size = 1280) {
  const image = createImage(size);
  const buffer = image.data;
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const offset = (y * size + x) * 4;
      buffer[offset] = 255;
      buffer[offset + 1] = 255;
      buffer[offset + 2] = 255;
      buffer[offset + 3] = 255;
    }
  }

  const slot = 520;
  placeScaledIcon(buffer, size, drawMainIcon(slot), 120, 140, slot);
  placeScaledIcon(buffer, size, drawDefaultEntryIcon(slot), 640, 140, slot);

  return image;
}

function placeScaledIcon(buffer, size, source, x, y, w) {
  for (let sy = 0; sy < source.size; sy++) {
    const dy = Math.min(size - 1, Math.max(0, y + sy));
    for (let sx = 0; sx < source.size; sx++) {
      const dx = Math.min(size - 1, Math.max(0, x + sx));
      const offset = (dy * size + dx) * 4;
      const srcOffset = (sy * source.size + sx) * 4;
      const a = source.data[srcOffset + 3] / 255;
      blendAt(buffer, offset, source.data.slice(srcOffset, srcOffset + 4), a);
    }
  }
}

function crc32(input) {
  let crc = 0xffffffff;
  for (let i = 0; i < input.length; i++) {
    crc ^= input[i];
    for (let j = 0; j < 8; j++) crc = crc & 1 ? 0xedb88320 ^ (crc >>> 1) : crc >>> 1;
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function chunk(type, data) {
  const typeBuf = Buffer.from(type, 'ascii');
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length, 0);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])), 0);
  return Buffer.concat([length, typeBuf, data, crc]);
}

// 过滤扫描行（filter 0 = None）：把每行开头的 filter 字节去掉并排成连续 RGBA。
function scanlinesToRgba(width, height, raw) {
  const stride = width * 4;
  const rgba = Buffer.alloc(stride * height);
  for (let y = 0; y < height; y++) {
    if (raw[y * (stride + 1)] !== 0) {
      throw new Error('unexpected non-zero filter byte in scanline ' + y);
    }
    raw.copy(rgba, y * stride, y * (stride + 1) + 1, (y + 1) * (stride + 1));
  }
  return rgba;
}

// APNG 编码：acTL + 帧 0(fcTL+IDAT) + 帧 1..n-1(fcTL+fdAT) + IEND。
// frames: [{ data: Buffer(连续 RGBA), delayNum?, delayDen? }]，默认 100ms/帧，无限循环。
function encodeApng(width, height, frames, defaultDelayNum = 10, defaultDelayDen = 100) {
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;

  const actl = Buffer.alloc(8);
  actl.writeUInt32BE(frames.length, 0);
  actl.writeUInt32BE(0, 4); // num_plays = 0 表示无限循环

  const parts = [
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('acTL', actl),
  ];

  let sequence = 0;
  frames.forEach((frame, index) => {
    const delayNum = frame.delayNum !== undefined ? frame.delayNum : defaultDelayNum;
    const delayDen = frame.delayDen !== undefined ? frame.delayDen : defaultDelayDen;

    const fctl = Buffer.alloc(26);
    fctl.writeUInt32BE(sequence++, 0); // sequence_number
    fctl.writeUInt32BE(width, 4);      // width
    fctl.writeUInt32BE(height, 8);     // height
    fctl.writeUInt32BE(0, 12);         // x_offset
    fctl.writeUInt32BE(0, 16);         // y_offset
    fctl.writeUInt16BE(delayNum, 20);  // delay_num
    fctl.writeUInt16BE(delayDen, 22);  // delay_den
    fctl[24] = 0;                      // dispose_op = 0 (none)
    fctl[25] = 0;                      // blend_op = 0 (source)
    parts.push(chunk('fcTL', fctl));

    const stride = width * 4;
    const raw = Buffer.alloc((stride + 1) * height);
    for (let y = 0; y < height; y++) {
      raw[y * (stride + 1)] = 0;
      frame.data.copy(raw, y * (stride + 1) + 1, y * stride, (y + 1) * stride);
    }
    const idat = zlib.deflateSync(raw, { level: 9 });

    if (index === 0) {
      parts.push(chunk('IDAT', idat));
    } else {
      const fdat = Buffer.alloc(4 + idat.length);
      fdat.writeUInt32BE(sequence++, 0); // fdAT 自带 sequence_number 前缀
      idat.copy(fdat, 4);
      parts.push(chunk('fdAT', fdat));
    }
  });

  parts.push(chunk('IEND', Buffer.alloc(0)));
  return Buffer.concat(parts);
}

function encodePng(width, height, rgba) {
  const stride = width * 4;
  const raw = Buffer.alloc((stride + 1) * height);
  for (let y = 0; y < height; y++) {
    raw[y * (stride + 1)] = 0;
    rgba.copy(raw, y * (stride + 1) + 1, y * stride, (y + 1) * stride);
  }
  const idat = zlib.deflateSync(raw, { level: 9 });

  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', idat),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

function writePng(filename, image) {
  const png = encodePng(image.size, image.size, image.data);
  fs.writeFileSync(path.join(OUT_DIR, filename), png);
}

// APNG 动画生成：24 帧 × 100ms = 2.4s 无限循环。
function writeApng(filename, drawFn, width, height, frameCount = 24) {
  const frames = [];
  for (let i = 0; i < frameCount; i++) {
    const image = drawFn(i / frameCount);
    frames.push({ data: image.data });
  }
  const apng = encodeApng(width, height, frames);
  fs.writeFileSync(path.join(OUT_DIR, filename), apng);
  return apng;
}

const ROOT_DIR = path.join(OUT_DIR, '..', 'shortcuts');
const APP_DIR = path.join(ROOT_DIR, 'app');
const APP_ICONS_DIR = path.join(APP_DIR, 'icons');
const UI_IMAGES_DIR = path.join(APP_DIR, 'ui', 'images');
const APNG_FRAMES = 24;

// 主图标：256px 输出为 APNG
const mainApng = writeApng('main_entry_256.png', (phase) => drawMainIcon(256, phase), 256, 256, APNG_FRAMES);
writePng('main_entry_64.png', drawMainIcon(256));   // 256px content, 64 filename

// 默认子应用图标：256px 输出为 APNG（交替脉动 + 内圆反相呼吸）
const childApng = writeApng('child_entry_256.png', (phase) => drawDefaultEntryIcon(256, phase), 256, 256, APNG_FRAMES);
writePng('child_entry_64.png', drawDefaultEntryIcon(256));   // 256px content, 64 filename

writePng('comparison.png', drawComparison(1280));

// 拷贝到 fnOS 需要的 3 个位置
// 1. 根目录：包图标
fs.writeFileSync(path.join(ROOT_DIR, 'ICON.PNG'), mainApng);
fs.writeFileSync(path.join(ROOT_DIR, 'ICON_256.PNG'), mainApng);

// 2. app/icons/：子应用默认图标
fs.mkdirSync(APP_ICONS_DIR, { recursive: true });
fs.copyFileSync(path.join(OUT_DIR, 'child_entry_256.png'), path.join(APP_ICONS_DIR, 'child_entry_256.png'));
fs.copyFileSync(path.join(OUT_DIR, 'child_entry_64.png'), path.join(APP_ICONS_DIR, 'child_entry_64.png'));

// 3. app/ui/images/：桌面图标
fs.mkdirSync(UI_IMAGES_DIR, { recursive: true });
fs.writeFileSync(path.join(UI_IMAGES_DIR, 'icon_256.png'), mainApng);
fs.writeFileSync(path.join(UI_IMAGES_DIR, 'icon_512.png'), mainApng);   // 256px content, 512 filename
fs.writeFileSync(path.join(UI_IMAGES_DIR, 'icon_64.png'), fs.readFileSync(path.join(OUT_DIR, 'main_entry_64.png')));
