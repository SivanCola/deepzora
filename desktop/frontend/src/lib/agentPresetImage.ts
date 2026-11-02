export const PRESET_ICON_IMAGE_LIMIT = 128 * 1024;
export const PRESET_ICON_SIZE = 128;
const PNG_PREFIX = "data:image/png;base64,";

export function isPresetIconImage(value: string): boolean {
  return value.length <= PRESET_ICON_IMAGE_LIMIT && value.startsWith(PNG_PREFIX)
    && /^[A-Za-z0-9+/]+={0,2}$/u.test(value.slice(PNG_PREFIX.length));
}

export function presetIconCrop(width: number, height: number, zoom: number, x: number, y: number) {
  const side = Math.min(width, height) / Math.max(1, Math.min(3, zoom));
  return { side, left: (width - side) * Math.max(0, Math.min(100, x)) / 100,
    top: (height - side) * Math.max(0, Math.min(100, y)) / 100 };
}

export async function decodePresetIcon(file: File): Promise<ImageBitmap> {
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) throw new Error("type");
  if (file.size > 5 * 1024 * 1024) throw new Error("size");
  const bitmap = await createImageBitmap(file).catch(() => { throw new Error("decode"); });
  if (!bitmap.width || !bitmap.height || bitmap.width > 8192 || bitmap.height > 8192) {
    bitmap.close();
    throw new Error("dimensions");
  }
  return bitmap;
}

export function drawPresetIcon(canvas: HTMLCanvasElement, source: ImageBitmap, zoom: number, x: number, y: number): void {
  const context = canvas.getContext("2d");
  if (!context) throw new Error("decode");
  const crop = presetIconCrop(source.width, source.height, zoom, x, y);
  context.clearRect(0, 0, canvas.width, canvas.height);
  context.imageSmoothingEnabled = true;
  context.imageSmoothingQuality = "high";
  context.drawImage(source, crop.left, crop.top, crop.side, crop.side, 0, 0, canvas.width, canvas.height);
}
