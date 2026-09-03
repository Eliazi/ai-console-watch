# Claude Console

i want to run some application from wsl that provide me the claude using and consuming using.. with more details that can improve my work
currently what im using is
#!/bin/bash




# Function to draw a color-coded progress bar

draw_bar() {

    local percent=$1

    local width=35 # Slightly narrower to fit the budget text

    local filled=$((percent * width / 100))

    local empty=$((width - filled))




    # Determine color based on threshold

    local color="\e[32m" # Green

    if [ "$percent" -ge 80 ]; then color="\e[31m" # Red

    elif [ "$percent" -ge 60 ]; then color="\e[33m" # Yellow

    fi




    printf "${color}"

    if [ "$filled" -gt 0 ]; then printf '█%.0s' $(seq 1 "$filled"); fi

    printf "\e[90m" # Dark grey for empty track

    if [ "$empty" -gt 0 ]; then printf '░%.0s' $(seq 1 "$empty"); fi

    printf "\e[0m"

}




# Hide the cursor for a cleaner look

tput civis

trap "tput cnorm; exit" SIGINT SIGTERM




while true; do

    # Budget Data (Currently Mock)

    BUDGET_LIMIT="50.00"

    CURRENT_SPEND="17.25"




    # Calculate budget percentage using awk (Bash can't do decimals natively)

    BUDGET_PERCENT=$(awk -v spend="$CURRENT_SPEND" -v limit="$BUDGET_LIMIT" 'BEGIN { printf "%d", (spend/limit)*100 }')




    # Render Dashboard

    clear

    echo -e "\e[36m╭─────────────────────────────────────────────────────────────╮\e[0m"

    echo -e "\e[36m│\e[0m                    \e[1mCLAUDE USAGE DASHBOARD\e[0m                   \e[36m│\e[0m"

    echo -e "\e[36m├─────────────────────────────────────────────────────────────┤\e[0m"

    echo -e "\e[36m│\e[0m                                                             \e[36m│\e[0m"




    # Monthly Budget Row

    BAR_BUDGET=$(draw_bar "$BUDGET_PERCENT")

    printf "\e[36m│\e[0m  Monthly Spend:   %b  \$%5s / \$%2s \e[36m│\e[0m\n" "$BAR_BUDGET" "$CURRENT_SPEND" "${BUDGET_LIMIT%.*}"




    echo -e "\e[36m│\e[0m                                                             \e[36m│\e[0m"

    echo -e "\e[36m╰─────────────────────────────────────────────────────────────╯\e[0m"

    echo -e "\e[90m  Press Ctrl+C to exit. Refreshing every 5 seconds...\e[0m"




    sleep 5

done

This project was built with [Lovable](https://lovable.dev).

## Build with Lovable

Continue developing this project in the [Lovable editor](https://lovable.dev/projects/1cc5b1da-7469-4f35-ad5f-1b7149ba8638).

- **Ship faster**: describe what you want to build and Lovable handles the code.
- **Stay in sync**: every change made in Lovable is committed straight to this repository.
- **Full ownership**: this code is yours. Push to `main` on GitHub and your changes sync back into Lovable, ready for your next prompt.

## Development

Prefer working locally? You need Node.js and npm — [install with nvm](https://github.com/nvm-sh/nvm#installing-and-updating).

```sh
git clone <this-repository-url>
cd <repository-name>
npm i
npm run dev
```
