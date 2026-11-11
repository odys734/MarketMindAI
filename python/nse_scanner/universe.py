"""Maintained Nifty 50 constituent snapshot; verify against official NSE files."""
from dataclasses import dataclass
@dataclass(frozen=True)
class Constituent:
    symbol: str
    sector: str
    sector_index: str
NIFTY50 = (
    Constituent("ADANIENT","Metals & Mining","^CNXMETAL"), Constituent("ADANIPORTS","Infrastructure","^CNXINFRA"),
    Constituent("APOLLOHOSP","Healthcare","^CNXPHARMA"), Constituent("ASIANPAINT","Consumer Durables","^CNXCONSUM"),
    Constituent("AXISBANK","Financial Services","^NSEBANK"), Constituent("BAJAJ-AUTO","Automobile","^CNXAUTO"),
    Constituent("BAJFINANCE","Financial Services","^CNXFIN"), Constituent("BAJAJFINSV","Financial Services","^CNXFIN"),
    Constituent("BEL","Capital Goods","^CNXINFRA"), Constituent("BHARTIARTL","Telecommunication","^CNXIT"),
    Constituent("CIPLA","Healthcare","^CNXPHARMA"), Constituent("COALINDIA","Metals & Mining","^CNXMETAL"),
    Constituent("DRREDDY","Healthcare","^CNXPHARMA"), Constituent("EICHERMOT","Automobile","^CNXAUTO"),
    Constituent("ETERNAL","Consumer Services","^CNXSERVICE"), Constituent("GRASIM","Cement & Building Materials","^CNXINFRA"),
    Constituent("HCLTECH","Information Technology","^CNXIT"), Constituent("HDFCBANK","Financial Services","^NSEBANK"),
    Constituent("HDFCLIFE","Financial Services","^CNXFIN"), Constituent("HEROMOTOCO","Automobile","^CNXAUTO"),
    Constituent("HINDALCO","Metals & Mining","^CNXMETAL"), Constituent("HINDUNILVR","FMCG","^CNXFMCG"),
    Constituent("ICICIBANK","Financial Services","^NSEBANK"), Constituent("INDUSINDBK","Financial Services","^NSEBANK"),
    Constituent("INFY","Information Technology","^CNXIT"), Constituent("ITC","FMCG","^CNXFMCG"),
    Constituent("JIOFIN","Financial Services","^CNXFIN"), Constituent("JSWSTEEL","Metals & Mining","^CNXMETAL"),
    Constituent("KOTAKBANK","Financial Services","^NSEBANK"), Constituent("LT","Capital Goods","^CNXINFRA"),
    Constituent("M&M","Automobile","^CNXAUTO"), Constituent("MARUTI","Automobile","^CNXAUTO"),
    Constituent("NESTLEIND","FMCG","^CNXFMCG"), Constituent("NTPC","Power","^CNXENERGY"),
    Constituent("ONGC","Oil & Gas","^CNXENERGY"), Constituent("POWERGRID","Power","^CNXENERGY"),
    Constituent("RELIANCE","Oil & Gas","^CNXENERGY"), Constituent("SBILIFE","Financial Services","^CNXFIN"),
    Constituent("SBIN","Financial Services","^NSEBANK"), Constituent("SHRIRAMFIN","Financial Services","^CNXFIN"),
    Constituent("SUNPHARMA","Healthcare","^CNXPHARMA"), Constituent("TATACONSUM","FMCG","^CNXFMCG"),
    Constituent("TATAMOTORS","Automobile","^CNXAUTO"), Constituent("TATASTEEL","Metals & Mining","^CNXMETAL"),
    Constituent("TCS","Information Technology","^CNXIT"), Constituent("TECHM","Information Technology","^CNXIT"),
    Constituent("TITAN","Consumer Durables","^CNXCONSUM"), Constituent("TRENT","Retail","^CNXCONSUM"),
    Constituent("ULTRACEMCO","Cement & Building Materials","^CNXINFRA"), Constituent("WIPRO","Information Technology","^CNXIT"),
)
NIFTY_TICKER = "^NSEI"
def yahoo_symbol(symbol: str) -> str:
    return f"{symbol}.NS"
