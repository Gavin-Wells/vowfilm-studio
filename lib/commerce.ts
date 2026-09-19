/** Commerce v3 generation, grouped by the configured model's task limit. */
export function isDirectCommerceV3(mode?: string) {
  if (!mode) return false;
  if (mode === 'commerce-direct-15s-v3') return true;
  return /^commerce-direct-\d+s-v3$/.test(mode);
}

export const commerceDurationOptions = () => {
  const out: number[] = [];
  for (let s = 10; s <= 60; s += 5) out.push(s);
  return out;
};

export function commerceDurationLabel(seconds: number) {
  return `${seconds} 秒`;
}
