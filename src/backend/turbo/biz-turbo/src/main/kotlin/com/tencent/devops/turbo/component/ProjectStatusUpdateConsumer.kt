package com.tencent.devops.turbo.component

import com.tencent.devops.turbo.pojo.ProjectEnableStatusBroadCastEvent
import com.tencent.devops.turbo.service.TurboPlanService
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.stereotype.Component

/**
 * 项目状态变更的队列消费者
 */
@Component
class ProjectStatusUpdateConsumer @Autowired constructor(
    private val turboPlanService: TurboPlanService
) {

    companion object {
        private val logger = LoggerFactory.getLogger(this::class.java)
    }

    fun consumer(event: ProjectEnableStatusBroadCastEvent) {
        try {
            with(event) {
                turboPlanService.updatePlanStatusByBkProjectStatus(
                    userId = userId,
                    projectId = projectId,
                    enabled = enabled
                )
            }
        } catch (e: Exception) {
            logger.error("batch update turbo plan status failed: ${e.message}", e)
        }
    }
}
