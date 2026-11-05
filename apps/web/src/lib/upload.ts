/** A signed upload instruction from the API. */
export type UploadInstruction = {
  url: string;
  method: string;
  headers: Record<string, string>;
};

export class UploadAborted extends Error {}

/**
 * Sends a file straight to object storage using a signed instruction.
 * Uses XMLHttpRequest because fetch can't report upload progress.
 */
export function sendFile(
  up: UploadInstruction,
  file: Blob,
  onProgress: (fraction: number) => void,
  signal: AbortSignal,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(up.method, up.url);
    for (const [name, value] of Object.entries(up.headers)) xhr.setRequestHeader(name, value);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else reject(new Error("The upload was refused by storage. Please try again."));
    };
    xhr.onerror = () => reject(new Error("The upload failed. Check your connection and try again."));
    xhr.onabort = () => reject(new UploadAborted("Upload cancelled."));
    signal.addEventListener("abort", () => xhr.abort(), { once: true });
    xhr.send(file);
  });
}

/** Largest file the API accepts in one upload (5 GiB). */
export const maxUploadBytes = 5 * 1024 ** 3;

/** What the file picker offers. The API decides from the file's contents. */
export const acceptedFiles = "video/*,audio/*,image/png,image/jpeg,image/webp,.mkv,.mov,.m4a,.flac";

export function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
}
