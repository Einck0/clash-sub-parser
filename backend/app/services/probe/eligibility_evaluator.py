from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

from app.schemas.probe_domain import ObservationEligibilityDTO, ProbeObservationDTO


def evaluate_observation_eligibility(
    observation: ProbeObservationDTO | dict[str, Any] | None,
    min_speed_mbps: float | None = None,
    required_media: list[str] | None = None,
    max_age_seconds: int | None = None,
) -> ObservationEligibilityDTO:
    """评估单条观测记录是否满足策略组筛选条件"""
    if observation is None:
        return ObservationEligibilityDTO(
            eligible=False,
            status="missing",
            reason="该节点尚无任何探测记录",
            observation=None,
        )

    if isinstance(observation, dict):
        obs_dto = ProbeObservationDTO(**observation)
    else:
        obs_dto = observation

    # 检查状态是否为失败
    if obs_dto.status in ("fail", "timeout"):
        return ObservationEligibilityDTO(
            eligible=False,
            status="failed",
            reason=f"最近一次探测失败: {obs_dto.error or obs_dto.status}",
            observation=obs_dto,
        )

    # 检查时效性
    if max_age_seconds is not None and max_age_seconds > 0:
        obs_time = obs_dto.observed_at
        if obs_time.tzinfo is None:
            obs_time = obs_time.replace(tzinfo=timezone.utc)
        now = datetime.now(timezone.utc)
        age = (now - obs_time).total_seconds()
        if age > max_age_seconds:
            return ObservationEligibilityDTO(
                eligible=False,
                status="stale",
                reason=f"探测记录已过期（已超过 {int(age)} 秒）",
                observation=obs_dto,
            )

    # 检查测速要求
    if min_speed_mbps is not None and min_speed_mbps > 0:
        spd = obs_dto.speed_mbps
        if spd is None or spd < min_speed_mbps:
            return ObservationEligibilityDTO(
                eligible=False,
                status="insufficient",
                reason=f"测速带宽不足（当前 {spd or 0.0} Mbps，要求 {min_speed_mbps} Mbps）",
                observation=obs_dto,
            )

    # 检查流媒体与 AI 解锁
    if required_media:
        media_map = obs_dto.media_results or {}
        for plat in required_media:
            p_key = plat.lower().strip()
            p_res = media_map.get(p_key)
            if not p_res:
                return ObservationEligibilityDTO(
                    eligible=False,
                    status="incompatible",
                    reason=f"缺少平台 {plat} 的探测项",
                    observation=obs_dto,
                )
            if isinstance(p_res, dict):
                p_status = p_res.get("status")
                unlocked = p_res.get("unlocked")
                if p_status not in ("ok", "full", "originals") and not unlocked:
                    return ObservationEligibilityDTO(
                        eligible=False,
                        status="insufficient",
                        reason=f"平台 {plat} 未能解锁（状态: {p_status}）",
                        observation=obs_dto,
                    )

    return ObservationEligibilityDTO(
        eligible=True,
        status="valid",
        reason="满足所有策略资格",
        observation=obs_dto,
    )
