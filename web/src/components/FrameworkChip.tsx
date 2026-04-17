/**
 * FrameworkChip — small pill showing a framework + section reference.
 * Example: "CIS-PG-16 §3.1"
 *
 * Used in the Controls table and the ControlDetailDrawer.
 */
export default function FrameworkChip({
  bundleName,
  section,
}: {
  bundleName: string;
  section: string;
}) {
  // Shorten common prefixes for compactness in table cells.
  const short = bundleName
    .replace(/^CIS\s+/i, 'CIS-')
    .replace(/\s+/g, '-');
  const label = section ? `${short} §${section}` : short;

  return (
    <span
      className="badge"
      style={{
        fontSize: 11,
        padding: '1px 6px',
        whiteSpace: 'nowrap',
        marginRight: 4,
        marginBottom: 2,
      }}
      title={`${bundleName} — section ${section}`}
    >
      {label}
    </span>
  );
}
