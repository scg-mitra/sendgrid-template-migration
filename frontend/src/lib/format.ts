/** Formats a count with a noun, e.g. plural(1, "template") is "1 template". */
export function plural(n: number, word: string, pluralWord = word + "s"): string {
  return `${n} ${n === 1 ? word : pluralWord}`
}
