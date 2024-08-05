#ifndef STATS_POST_H
#define STATS_POST_H

#include <stdint.h>

// Define structs for different stat types
typedef struct
{
    int packetsLost;
    uint64_t packetsReceived;
    uint64_t bitrate;
    unsigned int jitter;
} RtpSourceStats;

typedef struct
{
    uint64_t numLost;
    uint64_t numLate;
    uint64_t numDuplicates;
    uint64_t avgJitter;
    uint64_t rtxCount;
    uint64_t rtxSuccessCount;
    double rtxPerPacket;
    uint64_t rtxRtt;
} JitterBufferStats;

typedef struct
{
    uint64_t rtxDropCount;
    uint64_t sentNackCount;
    uint64_t recvNackCount;
} RtpSessionStats;

typedef enum
{
    RTP_SOURCE,
    JITTER_BUFFER,
    RTP_SESSION
} StatType;

typedef union
{
    RtpSourceStats rtpSourceStats;
    JitterBufferStats jitterBufferStats;
    RtpSessionStats rtpSessionStats;
} StatsUnion;

typedef struct
{
    StatType statType;
    StatsUnion stats;
} PostFields;

void sendPostRequest(PostFields postFields, const char *statTypeStr, const char *hostname, const char *cameraID);

#endif // STATS_POST_H
