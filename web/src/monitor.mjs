export function formatMetric(value, unit = 'bytes') {
 if (value == null || !Number.isFinite(value)) return '—';
 if (unit === 'percent') return `${value.toLocaleString('en-US', {maximumFractionDigits:1})}%`;
 const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
 let index = 0;
 while (value >= 1024 && index < units.length-1) {value /= 1024;index++;}
 return `${value.toLocaleString('en-US', {maximumFractionDigits:index ? 1 : 0})} ${units[index]}${unit === 'rate' ? '/s' : ''}`;
}
// CPU capacity is 100 for the host and 100 × host CPUs for Docker's per-core metric.
// Unknown limits and missing samples must never appear as healthy zero usage.
export function resourceLoad(value, capacity) {
 if (!Number.isFinite(value) || value < 0 || !Number.isFinite(capacity) || capacity <= 0) {
  return {level:'unknown', label:'No data', percent:null, width:0};
 }
 const percent = value / capacity * 100;
 const level = percent >= 90 ? 'danger' : percent >= 70 ? 'warning' : 'normal';
 return {level, label:{normal:'Normal', warning:'Elevated', danger:'High'}[level], percent, width:Math.min(100,percent)};
}

export function chartPath(points, key, start, end, maximum) {
 let path = '', previous = null;
 for (const p of points) {
  const value = p[key];
  if (value == null || !Number.isFinite(value)) {previous = null;continue;}
  const x = 48 + 494 * (p.time - start) / Math.max(1,end-start);
  const y = 106 - 96 * Math.max(0,Math.min(1,value/maximum));
  path += `${previous === null || p.time-previous > 15000 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)} `;
  previous = p.time;
 }
 return path.trim();
}
