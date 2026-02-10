#!/bin/bash

# Function to generate random user data
generate_user_data() {
    name=$(cat /dev/urandom | tr -dc 'a-zA-Z' | fold -w 10 | head -n 1)
    email="${name}@example.com"
    password=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 12 | head -n 1)
    echo "{\"name\":\"$name\",\"email\":\"$email\",\"password\":\"$password\"}"
}

# Function to send requests and capture performance
send_requests() {
    endpoint=$1
    num_requests=$2
    concurrency=$3

    start_time=$(date +%s.%N)

    for i in $(seq 1 $num_requests); do
        user_data=$(generate_user_data)
        curl -s -X POST -H "Content-Type: application/json" -d "$user_data" "http://localhost:8080$endpoint" > /dev/null &
        
        if [ $((i % concurrency)) -eq 0 ]; then
            wait
        fi
    done

    wait

    end_time=$(date +%s.%N)
    duration=$(echo "$end_time - $start_time" | bc -l)
    requests_per_second=$(echo "$num_requests / $duration" | bc -l)

    echo "$duration $requests_per_second"
}

# Test parameters
num_requests=50
concurrency=1

# Run tests and capture results
echo "Testing without load balancer..."
results_no_lb=$(send_requests "/register" $num_requests $concurrency)

echo "Testing with load balancer..."
results_lb=$(send_requests "/register-lb" $num_requests $concurrency)

# Extract the relevant data
duration_no_lb=$(echo $results_no_lb | awk '{print $1}')
rps_no_lb=$(echo $results_no_lb | awk '{print $2}')

duration_lb=$(echo $results_lb | awk '{print $1}')
rps_lb=$(echo $results_lb | awk '{print $2}')

# Calculate times differences
time_times_difference=$(echo "$duration_no_lb / $duration_lb" | bc -l)
rps_times_difference=$(echo "$rps_lb / $rps_no_lb" | bc -l)

# Display the comparison
echo "Comparison:"
echo "-----------------------------------"
echo "Without Load Balancer: "
printf "  Total time: %.2f seconds\n" "$duration_no_lb"
printf "  Requests per second: %.2f\n" "$rps_no_lb"
echo
echo "With Load Balancer: "
printf "  Total time: %.2f seconds\n" "$duration_lb"
printf "  Requests per second: %.2f\n" "$rps_lb"
echo
echo "Difference:"
echo "-----------------------------------"
printf "Time difference: %.2f seconds\n" "$(echo "$duration_no_lb - $duration_lb" | bc -l)"
printf "Requests per second difference: %.2f\n" "$(echo "$rps_lb - $rps_no_lb" | bc -l)"
echo
echo "Times Difference:"
echo "-----------------------------------"
printf "Time is faster by: %.2f times\n" "$time_times_difference"
printf "Requests per second increased by: %.2f times\n" "$rps_times_difference"
