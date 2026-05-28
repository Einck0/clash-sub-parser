from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.schemas.node_group import (
    NodeGroupCreate,
    NodeGroupRead,
    NodeGroupReorder,
    NodeGroupUpdate,
)
from app.services.node_group_service import (
    create_node_group,
    delete_node_group,
    get_node_group,
    list_node_groups,
    preview_node_groups,
    reorder_node_groups,
    update_node_group,
    validate_node_groups,
)

router = APIRouter(prefix="/node-groups", tags=["node-groups"])


@router.get("", response_model=list[NodeGroupRead])
async def get_node_groups(db: AsyncSession = Depends(get_db)) -> list[NodeGroupRead]:
    return await list_node_groups(db)


@router.post("", response_model=NodeGroupRead, status_code=status.HTTP_201_CREATED)
async def create_node_group_endpoint(
    payload: NodeGroupCreate,
    db: AsyncSession = Depends(get_db),
) -> NodeGroupRead:
    return await create_node_group(db, payload)


@router.patch("/{node_group_id}", response_model=NodeGroupRead)
async def update_node_group_endpoint(
    node_group_id: int,
    payload: NodeGroupUpdate,
    db: AsyncSession = Depends(get_db),
) -> NodeGroupRead:
    item = await get_node_group(db, node_group_id)
    if not item:
        raise HTTPException(status_code=404, detail="Node group not found")
    return await update_node_group(db, item, payload)


@router.delete("/{node_group_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_node_group_endpoint(
    node_group_id: int, db: AsyncSession = Depends(get_db)
) -> None:
    item = await get_node_group(db, node_group_id)
    if not item:
        raise HTTPException(status_code=404, detail="Node group not found")
    await delete_node_group(db, item)


@router.post("/reorder", response_model=list[NodeGroupRead])
async def reorder_node_group_endpoint(
    payload: NodeGroupReorder,
    db: AsyncSession = Depends(get_db),
) -> list[NodeGroupRead]:
    return await reorder_node_groups(db, payload)


@router.post("/validate")
async def validate_node_group_endpoint(
    db: AsyncSession = Depends(get_db),
) -> dict[str, str]:
    return await validate_node_groups(db)


@router.get("/_preview")
async def preview_node_group_endpoint(
    db: AsyncSession = Depends(get_db),
) -> list[dict]:
    return await preview_node_groups(db)
