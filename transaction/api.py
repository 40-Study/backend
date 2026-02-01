import mbbank
import datetime
import os
from fastapi import FastAPI, Query, HTTPException
from pydantic import BaseModel
from typing import Optional, List

app = FastAPI(title="MBBank Transaction API")

USERNAME = os.getenv("MB_USERNAME", "")
PASSWORD = os.getenv("MB_PASSWORD", "")
ACCOUNT_NO = os.getenv("MB_ACCOUNT_NO", "")

class TransactionDetail(BaseModel):
    posting_date: Optional[str]
    transaction_date: Optional[str]
    account_no: Optional[str]
    credit_amount: Optional[str]
    debit_amount: Optional[str]
    currency: Optional[str]
    description: Optional[str]
    add_description: Optional[str]
    available_balance: Optional[str]
    beneficiary_account: Optional[str]
    ref_no: Optional[str]
    ben_account_name: Optional[str]
    bank_name: Optional[str]
    ben_account_no: Optional[str]
    transaction_type: Optional[str]

# hàm chuyển đổi ngày tháng string
def parse_datetime(datetime_str):
    try:
        parts = datetime_str.split('-')
        if len(parts) != 6:
            raise ValueError("Invalid format. Expected: hh-mm-ss-dd-mm-yyyy")
        
        hour = int(parts[0])
        minute = int(parts[1])
        second = int(parts[2])
        day = int(parts[3])
        month = int(parts[4])
        year = int(parts[5])
        
        return datetime.datetime(year, month, day, hour, minute, second)
    except Exception as e:
        raise ValueError(f"Invalid datetime format: {str(e)}")

def extract_transaction_data(trans):
    return {
        "posting_date": getattr(trans, 'postingDate', None),
        "transaction_date": getattr(trans, 'transactionDate', None),
        "account_no": getattr(trans, 'accountNo', None),
        "credit_amount": getattr(trans, 'creditAmount', None),
        "debit_amount": getattr(trans, 'debitAmount', None),
        "currency": getattr(trans, 'currency', None),
        "description": getattr(trans, 'description', None),
        "add_description": getattr(trans, 'addDescription', None),
        "available_balance": getattr(trans, 'availableBalance', None),
        "beneficiary_account": getattr(trans, 'beneficiaryAccount', None),
        "ref_no": getattr(trans, 'refNo', None),
        "ben_account_name": getattr(trans, 'benAccountName', None),
        "bank_name": getattr(trans, 'bankName', None),
        "ben_account_no": getattr(trans, 'benAccountNo', None),
        "transaction_type": getattr(trans, 'transactionType', None)
    }

@app.get("/transactions")
async def get_transactions(
    from_date: str = Query(..., description="Format: hh-mm-ss-dd-mm-yyyy"),
    to_date: str = Query(..., description="Format: hh-mm-ss-dd-mm-yyyy")
):
    try:
        from_dt = parse_datetime(from_date)
        to_dt = parse_datetime(to_date)
        
        if from_dt > to_dt:
            raise HTTPException(status_code=400, detail="from_date must be before or equal to to_date")
        
        mb = mbbank.MBBank(username=USERNAME, password=PASSWORD)
        
        transactions = mb.getTransactionAccountHistory(
            accountNo=ACCOUNT_NO,
            from_date=from_dt,
            to_date=to_dt
        )
        
        transaction_list = []
        if hasattr(transactions, 'transactionHistoryList') and transactions.transactionHistoryList:
            for trans in transactions.transactionHistoryList:
                transaction_list.append(extract_transaction_data(trans))
        
        return {
            "success": True,
            "transaction_count": len(transaction_list),
            "from_date": from_dt.strftime("%Y-%m-%d %H:%M:%S"),
            "to_date": to_dt.strftime("%Y-%m-%d %H:%M:%S"),
            "transactions": transaction_list
        }
        
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"An error occurred: {str(e)}")

@app.get("/transactions/count")
async def get_transaction_count(
    from_date: str = Query(..., description="Format: hh-mm-ss-dd-mm-yyyy"),
    to_date: str = Query(..., description="Format: hh-mm-ss-dd-mm-yyyy")
):
    try:
        from_dt = parse_datetime(from_date)
        to_dt = parse_datetime(to_date)
        
        if from_dt > to_dt:
            raise HTTPException(status_code=400, detail="from_date must be before or equal to to_date")
        
        mb = mbbank.MBBank(username=USERNAME, password=PASSWORD)
        
        transactions = mb.getTransactionAccountHistory(
            accountNo=ACCOUNT_NO,
            from_date=from_dt,
            to_date=to_dt
        )
        
        count = 0
        if hasattr(transactions, 'transactionHistoryList') and transactions.transactionHistoryList:
            count = len(transactions.transactionHistoryList)
        
        return {
            "success": True,
            "transaction_count": count,
            "from_date": from_dt.strftime("%Y-%m-%d %H:%M:%S"),
            "to_date": to_dt.strftime("%Y-%m-%d %H:%M:%S")
        }
        
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"An error occurred: {str(e)}")

@app.get("/transactions/check-pin")
async def check_transaction_by_pin(
    pin: str = Query(..., description="Mã PIN 15 ký tự (9 chữ cái + 6 ký tự mã hóa thời gian)", min_length=15, max_length=15),
    from_date: str = Query(..., description="Format: hh-mm-ss-dd-mm-yyyy"),
    to_date: str = Query(..., description="Format: hh-mm-ss-dd-mm-yyyy")
):
    try:
        if len(pin) != 15:
            raise HTTPException(status_code=400, detail="PIN must be exactly 15 characters")
        
        from_dt = parse_datetime(from_date)
        to_dt = parse_datetime(to_date)
        
        if from_dt > to_dt:
            raise HTTPException(status_code=400, detail="from_date must be before or equal to to_date")
        
        mb = mbbank.MBBank(username=USERNAME, password=PASSWORD)
        
        transactions = mb.getTransactionAccountHistory(
            accountNo=ACCOUNT_NO,
            from_date=from_dt,
            to_date=to_dt
        )
        
        matching_transactions = []
        if hasattr(transactions, 'transactionHistoryList') and transactions.transactionHistoryList:
            for trans in transactions.transactionHistoryList:
                description = getattr(trans, 'description', '')
                add_description = getattr(trans, 'addDescription', '')
                
                if pin in description or pin in add_description:
                    matching_transactions.append(extract_transaction_data(trans))
        
        if matching_transactions:
            return {
                "success": True,
                "found": True,
                "pin": pin,
                "match_count": len(matching_transactions),
                "transactions": matching_transactions
            }
        else:
            return {
                "success": True,
                "found": False,
                "pin": pin,
                "message": "No transaction found with this PIN"
            }
        
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"An error occurred: {str(e)}")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)