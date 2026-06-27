from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession
from app.database import get_db
from app.schemas.config_snapshot import ConfigSnapshotCreate, ConfigSnapshotRead
from app.services.snapshot_service import create_snapshot, list_snapshots, get_snapshot, restore_snapshot

router = APIRouter(prefix="/snapshots", tags=["snapshots"])

@router.get("", response_model=list[ConfigSnapshotRead])
async def list_snapshots_endpoint(db: AsyncSession = Depends(get_db)):
    return await list_snapshots(db)

@router.post("", response_model=ConfigSnapshotRead)
async def create_snapshot_endpoint(
    payload: ConfigSnapshotCreate, db: AsyncSession = Depends(get_db)
):
    return await create_snapshot(db, label=payload.label, description=payload.description)

@router.get("/{snapshot_id}", response_model=ConfigSnapshotRead)
async def get_snapshot_endpoint(snapshot_id: int, db: AsyncSession = Depends(get_db)):
    item = await get_snapshot(db, snapshot_id)
    if not item:
        raise HTTPException(status_code=404, detail="Snapshot not found")
    return item

@router.get("/{snapshot_id}/data")
async def get_snapshot_data_endpoint(snapshot_id: int, db: AsyncSession = Depends(get_db)):
    import json
    item = await get_snapshot(db, snapshot_id)
    if not item:
        raise HTTPException(status_code=404, detail="Snapshot not found")
    return json.loads(item.snapshot_data)

@router.post("/{snapshot_id}/restore")
async def restore_snapshot_endpoint(snapshot_id: int, db: AsyncSession = Depends(get_db)):
    try:
        return await restore_snapshot(db, snapshot_id)
    except ValueError as e:
        raise HTTPException(status_code=404, detail=str(e))

@router.delete("/{snapshot_id}")
async def delete_snapshot_endpoint(snapshot_id: int, db: AsyncSession = Depends(get_db)):
    item = await get_snapshot(db, snapshot_id)
    if item:
        await db.delete(item)
        await db.commit()
    return {"ok": True}
