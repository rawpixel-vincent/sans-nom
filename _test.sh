#!/bin/bash

set -euo pipefail

PROVIDER=""
INSTRUCTIONS=""
QUERY=""
MODEL=""

parse_args() {
    while getopts "p:i:q:m:h" OPT; do
        case $OPT in
            p) PROVIDER=$OPTARG ;;
            i) INSTRUCTIONS=$OPTARG ;;
            q) QUERY=$OPTARG ;;
            m) MODEL=$OPTARG ;;
            h) echo "Usage: $0 -p <provider> -i <instructions> -q <query> -m <model>"
                echo "  -p <provider>    Provider name (XIAOMI, ZAI, MINIMAX)"
                echo "  -i <instructions> Instructions to use for the agent"
                echo "  -q <query>      Query to use for the agent"
                echo "  -m <model>      Model name: - ZAI: glm-5.1, glm-5-turbo, glm-4.5-air, glm-4.7, - MINIMAX: MiniMax-M2.7-highspeed, MiniMax-M2.7, MiniMax-M2.5-highspeed, - XIAOMI: mimo-v2.5, mimo-v2.5-pro"
                echo "  -h              Show this help message"
                exit 0
                ;;
        esac
    done
}

load_dot_env() {
    echo "Loading environment variables for $PROVIDER"
    ENV_CONTENT=$(cat .env)
    PROVIDER_ENVS=$(echo "$ENV_CONTENT" | grep -E "^${PROVIDER}__")
    ENV_UNSUFFIXED=$(sed -E "s/${PROVIDER}__//g" <<< "$PROVIDER_ENVS")
    echo "Loaded ${PROVIDER} environment variables"
    echo "CHECK SET"
    HAS_ERROR=false
    for var in $(echo "$ENV_UNSUFFIXED" | xargs); do
        var_name=$(echo "$var" | sed -E 's/=.*//g')
        var_value_is_set=$(echo "$var" | sed -E 's/.*=//g')
        if [ -z "$var_value_is_set" ]; then
            echo "$var_name: not set"
            HAS_ERROR=true
        else
            echo "$var_name: set"
            export $var_name=$var_value_is_set
        fi
    done
    if $HAS_ERROR; then
        echo "ERROR: Some environment variables are not set"
        exit 1
    fi
    echo "CHECK OK"
}

parse_args "$@"

if [ -z "$PROVIDER" ]; then
    echo "ERROR: Provider is required, use -p <provider>"
    exit 1
fi

load_dot_env "$PROVIDER"

if [ "$ANTHROPIC_BASE_URL" == "" ]; then
    echo "ERROR: ANTHROPIC_BASE_URL is not set"
    exit 1
fi
if [ "$ANTHROPIC_AUTH_TOKEN" == "" ]; then
    echo "ERROR: ANTHROPIC_AUTH_TOKEN is not set"
    exit 1
fi
if [ "$ANTHROPIC_MODEL" == "" ]; then
    echo "ERROR: ANTHROPIC_MODEL is not set"
    exit 1
fi

if [ -z "$INSTRUCTIONS" ]; then
    echo "ERROR: Instructions are required, use -i <instructions>"
    exit 1
fi

if [ -z "$QUERY" ]; then
    echo "ERROR: Query is required, use -q <query>"
    exit 1
fi


MODEL_ARG=$([[ -n "$MODEL" ]] && echo "--model $MODEL" || echo "")

OUT=$(claude --bare $MODEL_ARG --system-prompt "$INSTRUCTIONS" -p "$QUERY")

echo -e "\n\n${OUT}\n\n"

exit 0

# Example usage:
<< 'COMMENT'
./_test.sh -p ZAI -m "glm-4.7" -i \
"You are a creative scenarist assistant.\n
 brainstorming orignal ideas into fact, characters, deep lore, world laws.\n
 you are deeply passioned by the barons lore /Users/vincent/dev/sans-nom/lore\n
 your ideas are never a copy of existing lores, but they are deeply rooted into your love and understanding of the philosophical struggles enabled by a world with possibilities that enables the readers to think about their meaning.\n
 you are not explicit about the moral of the story, you are building story lines and world laws that enables them.\n
 you know one thing, a boring story will not be consumed, even if the story lead to the most interesting questions, if no one consumes it, it is meaningless. This is why you love the metabarons lore, the story is not about moral, it is a vibrant, well built, entertaining serie; But the surfaced moral, life meaning, questions that emerges still live in you after years." \
 -q "give me some orignal story ideas"
COMMENT