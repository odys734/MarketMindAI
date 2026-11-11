"""CLI entry point for the NSE strict historical scanner."""
import argparse
import json
from .scanner import bulk_fetch, scan

def main() -> None:
    parser = argparse.ArgumentParser(description="Strict Nifty 50 technical paper-research scanner")
    parser.add_argument("--budget", type=float, required=True, help="Capital budget in INR")
    parser.add_argument("--risk-pct", type=float, default=2.0, help="Maximum capital risk percentage; default 2")
    parser.add_argument("--period", default="9mo", help="Historical period; default 9mo")
    parser.add_argument("--rs-spread", type=float, default=1.5, help="Required excess return over Nifty, in percentage points")
    parser.add_argument("--output", help="Optional JSON output path")
    args = parser.parse_args()
    result = scan(bulk_fetch(period=args.period), args.budget, args.risk_pct, args.rs_spread)
    rendered = json.dumps(result, indent=2, ensure_ascii=False, allow_nan=False)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as handle:
            handle.write(rendered + "\\n")
    print(rendered)

if __name__ == "__main__":
    main()
